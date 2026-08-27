package portablebundle

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/luoyif/memory-harness/internal/contracts"
	"github.com/luoyif/memory-harness/internal/harness"
	"github.com/luoyif/memory-harness/internal/ledger"
	"github.com/luoyif/memory-harness/internal/portfolio"
	"github.com/luoyif/memory-harness/internal/store"
)

type Service struct {
	control *store.ControlStore
	harness *harness.Service
	ledger  *ledger.Ledger
}

func New(control *store.ControlStore, harnessService *harness.Service, evidence *ledger.Ledger) *Service {
	return &Service{control: control, harness: harnessService, ledger: evidence}
}
func digest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func capabilityTags(record ObjectRecord) []string {
	out := []string{"object-type:" + record.TypeID}
	plugins := map[string]bool{}
	for _, revision := range record.Revisions {
		if revision.PluginID != "" {
			// Capability negotiation is semantic, not vendor-version identity.
			// The exact plugin version remains pinned on every Revision record.
			plugins["plugin:"+revision.PluginID] = true
		}
	}
	for value := range plugins {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func presentationHint(typeID string) string {
	switch {
	case strings.Contains(typeID, "profile"):
		return "profile"
	case strings.Contains(typeID, "knowledge-product"):
		return "document"
	case strings.Contains(typeID, "asset"):
		return "agent_asset"
	default:
		return "structured_json"
	}
}
func (s *Service) objectIDs(ctx context.Context, projectID string) ([]string, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT object_id FROM harness_objects WHERE project_id=? ORDER BY object_id`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Service) revisions(ctx context.Context, object harness.Object) ([]RevisionRecord, map[string]json.RawMessage, error) {
	rows, err := s.control.DB.QueryContext(ctx, `SELECT revision,status,schema_version,payload_json,content_hash,confidence,importance,valid_from,valid_until,source_evidence_ids_json,source_object_ids_json,run_id,stage_id,plugin_id,plugin_version,created_at FROM harness_object_revisions WHERE object_id=? ORDER BY revision`, object.ObjectID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	records := []RevisionRecord{}
	payloads := map[string]json.RawMessage{}
	for rows.Next() {
		var r RevisionRecord
		var payload, evidenceRaw, objectsRaw string
		var validUntil, runID, stageID sql.NullString
		if err := rows.Scan(&r.Revision, &r.Status, &r.SchemaVersion, &payload, &r.ContentHash, &r.Confidence, &r.Importance, &r.ValidFrom, &validUntil, &evidenceRaw, &objectsRaw, &runID, &stageID, &r.PluginID, &r.PluginVersion, &r.CreatedAt); err != nil {
			return nil, nil, err
		}
		r.ValidUntil, r.RunID, r.StageID = validUntil.String, runID.String, stageID.String
		_ = json.Unmarshal([]byte(evidenceRaw), &r.SourceEvidenceIDs)
		_ = json.Unmarshal([]byte(objectsRaw), &r.SourceObjectIDs)
		r.BlobSHA256 = digest([]byte(payload))
		records = append(records, r)
		payloads[strconv.Itoa(r.Revision)] = json.RawMessage(payload)
	}
	return records, payloads, rows.Err()
}
func (s *Service) recordForObject(ctx context.Context, object harness.Object) (ObjectRecord, map[string]json.RawMessage, error) {
	if object.TypeID == ImportCandidateTypeV1 {
		var candidate ImportCandidate
		if err := json.Unmarshal(object.Revision.Payload, &candidate); err != nil {
			return ObjectRecord{}, nil, err
		}
		candidate.OriginalObject.ProjectID = candidate.OriginalProjectID
		return candidate.OriginalObject, candidate.RevisionPayloads, nil
	}
	revisions, payloads, err := s.revisions(ctx, object)
	if err != nil {
		return ObjectRecord{}, nil, err
	}
	record := ObjectRecord{
		ObjectID: object.ObjectID, TypeID: object.TypeID, ProjectID: object.ProjectID,
		Status: object.Status, ProtectionClass: object.ProtectionClass,
		CurrentRevision: object.CurrentRevision, CreatedAt: object.CreatedAt, UpdatedAt: object.UpdatedAt,
		PresentationHint: presentationHint(object.TypeID), Revisions: revisions,
	}
	record.Capabilities = capabilityTags(record)
	return record, payloads, nil
}

func dependencyIDs(record ObjectRecord) []string {
	out := []string{}
	for _, revision := range record.Revisions {
		out = append(out, revision.SourceObjectIDs...)
	}
	return unique(out)
}

func evidenceIDs(record ObjectRecord) []string {
	out := []string{}
	for _, revision := range record.Revisions {
		out = append(out, revision.SourceEvidenceIDs...)
	}
	return unique(out)
}
func (s *Service) collectSelection(ctx context.Context, options ExportOptions) ([]ObjectRecord, map[string]json.RawMessage, []EvidenceRecord, map[string][]byte, error) {
	options.ProjectID = strings.TrimSpace(options.ProjectID)
	if options.ProjectID == "" {
		return nil, nil, nil, nil, errors.New("project_id is required")
	}
	allIDs, err := s.objectIDs(ctx, options.ProjectID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	allowed := map[string]bool{}
	for _, id := range allIDs {
		allowed[id] = true
	}
	queue := unique(options.ObjectIDs)
	if len(queue) == 0 {
		queue = append(queue, allIDs...)
	}
	selected := map[string]bool{}
	records := []ObjectRecord{}
	payloads := map[string]json.RawMessage{}
	evidenceSet := map[string]bool{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if selected[id] {
			continue
		}
		if !allowed[id] {
			return nil, nil, nil, nil, fmt.Errorf("object %s is outside project %s", id, options.ProjectID)
		}
		object, err := s.harness.Object(ctx, id)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		record, objectPayloads, err := s.recordForObject(ctx, object)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		selected[id] = true
		records = append(records, record)
		for revision, payload := range objectPayloads {
			payloads[record.ObjectID+"@"+revision] = payload
		}
		for _, evidenceID := range evidenceIDs(record) {
			evidenceSet[evidenceID] = true
		}
		if options.IncludeDependencies {
			for _, dep := range dependencyIDs(record) {
				if allowed[dep] && !selected[dep] {
					queue = append(queue, dep)
				}
			}
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ObjectID < records[j].ObjectID })
	return s.collectEvidence(ctx, records, payloads, evidenceSet)
}
func (s *Service) collectEvidence(ctx context.Context, records []ObjectRecord, payloads map[string]json.RawMessage, ids map[string]bool) ([]ObjectRecord, map[string]json.RawMessage, []EvidenceRecord, map[string][]byte, error) {
	blobs := map[string][]byte{}
	for _, payload := range payloads {
		raw := []byte(payload)
		blobs[digest(raw)] = append([]byte(nil), raw...)
	}
	evidence := []EvidenceRecord{}
	for id := range ids {
		raw, err := s.ledger.ReadEvidence(ctx, id)
		if err != nil {
			return nil, nil, nil, nil, fmt.Errorf("read Evidence %s: %w", id, err)
		}
		receipt, ok, err := s.control.Receipt(ctx, id)
		if err != nil || !ok {
			return nil, nil, nil, nil, fmt.Errorf("Evidence %s receipt missing", id)
		}
		hash := digest(raw)
		blobs[hash] = append([]byte(nil), raw...)
		evidence = append(evidence, EvidenceRecord{EvidenceID: id, BlobSHA256: hash, LineHash: receipt.LineHash, SourceSystem: receipt.SourceSystem, SessionID: receipt.SessionID, ObservedAt: receipt.ObservedAt, CapturedAt: receipt.CapturedAt})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].EvidenceID < evidence[j].EvidenceID })
	return records, payloads, evidence, blobs, nil
}

func jsonLines[T any](items []T) ([]byte, error) {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func requiredCapabilities(objects []ObjectRecord, evidence []EvidenceRecord) []string {
	out := []string{}
	if len(evidence) > 0 {
		out = append(out, "evidence:v1")
	}
	for _, object := range objects {
		out = append(out, object.Capabilities...)
	}
	return unique(out)
}
func bundleDigest(objectsRaw, evidenceRaw []byte, blobs map[string][]byte) string {
	h := sha256.New()
	_, _ = h.Write(objectsRaw)
	_, _ = h.Write(evidenceRaw)
	keys := make([]string, 0, len(blobs))
	for key := range blobs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		_, _ = h.Write([]byte(key))
		_, _ = h.Write(blobs[key])
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func writeTarEntry(tw *tar.Writer, name string, raw []byte) error {
	header := &tar.Header{Name: name, Mode: 0600, Size: int64(len(raw)), ModTime: time.Unix(0, 0).UTC()}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(raw)
	return err
}

func (s *Service) Export(ctx context.Context, options ExportOptions, outputPath string) (Manifest, error) {
	objects, _, evidence, blobs, err := s.collectSelection(ctx, options)
	if err != nil {
		return Manifest{}, err
	}
	objectsRaw, err := jsonLines(objects)
	if err != nil {
		return Manifest{}, err
	}
	evidenceRaw, err := jsonLines(evidence)
	if err != nil {
		return Manifest{}, err
	}
	bundleHash := bundleDigest(objectsRaw, evidenceRaw, blobs)
	manifest := Manifest{SchemaVersion: SchemaVersion, BundleID: "bundle-" + strings.TrimPrefix(bundleHash, "sha256:")[:24], CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), SourceProjectID: strings.TrimSpace(options.ProjectID), Selection: Selection{RootObjectIDs: unique(options.ObjectIDs), IncludeDependencies: options.IncludeDependencies}, ObjectCount: len(objects), EvidenceCount: len(evidence), RequiredCapabilities: requiredCapabilities(objects, evidence), BundleHash: bundleHash, Signature: Signature{Status: "unsigned", Algorithm: "none"}}
	manifestRaw, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o700); err != nil {
		return Manifest{}, err
	}
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Manifest{}, err
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)
	fail := func(e error) (Manifest, error) { _ = tw.Close(); _ = gz.Close(); _ = out.Close(); return Manifest{}, e }
	if err := writeTarEntry(tw, "bundle-manifest.json", manifestRaw); err != nil {
		return fail(err)
	}
	if err := writeTarEntry(tw, "objects.jsonl", objectsRaw); err != nil {
		return fail(err)
	}
	if err := writeTarEntry(tw, "evidence.jsonl", evidenceRaw); err != nil {
		return fail(err)
	}
	keys := make([]string, 0, len(blobs))
	for key := range blobs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writeTarEntry(tw, "blobs/sha256/"+key+".json", blobs[key]); err != nil {
			return fail(err)
		}
	}
	if err := tw.Close(); err != nil {
		return fail(err)
	}
	if err := gz.Close(); err != nil {
		_ = out.Close()
		return Manifest{}, err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return Manifest{}, err
	}
	return manifest, out.Close()
}

type loadedBundle struct {
	Manifest Manifest
	Objects  []ObjectRecord
	Evidence []EvidenceRecord
	Blobs    map[string][]byte
}

func safeArchiveName(name string) bool {
	clean := filepath.ToSlash(filepath.Clean(name))
	return clean != "." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
}
func decodeJSONL[T any](raw []byte) ([]T, error) {
	items := []T{}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64<<10), 16<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, scanner.Err()
}

func loadBundle(path string) (loadedBundle, error) {
	file, err := os.Open(path)
	if err != nil {
		return loadedBundle{}, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return loadedBundle{}, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	parts := map[string][]byte{}
	total := int64(0)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return loadedBundle{}, err
		}
		if header.Typeflag != tar.TypeReg || !safeArchiveName(header.Name) {
			return loadedBundle{}, fmt.Errorf("unsafe bundle entry %q", header.Name)
		}
		if header.Size < 0 || header.Size > 16<<20 {
			return loadedBundle{}, fmt.Errorf("bundle entry %q exceeds 16 MiB", header.Name)
		}
		total += header.Size
		if total > 256<<20 {
			return loadedBundle{}, errors.New("bundle exceeds 256 MiB uncompressed limit")
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		allowed := name == "bundle-manifest.json" || name == "objects.jsonl" || name == "evidence.jsonl" || strings.HasPrefix(name, "blobs/sha256/")
		if !allowed {
			return loadedBundle{}, fmt.Errorf("unsupported bundle entry %q", name)
		}
		raw, err := io.ReadAll(io.LimitReader(tr, header.Size+1))
		if err != nil {
			return loadedBundle{}, fmt.Errorf("read bundle entry %q: %w", name, err)
		}
		if int64(len(raw)) != header.Size {
			return loadedBundle{}, fmt.Errorf("short bundle entry %q", name)
		}
		if _, exists := parts[name]; exists {
			return loadedBundle{}, fmt.Errorf("duplicate bundle entry %q", name)
		}
		parts[name] = raw
	}
	var manifest Manifest
	if err := json.Unmarshal(parts["bundle-manifest.json"], &manifest); err != nil {
		return loadedBundle{}, errors.New("missing or invalid bundle manifest")
	}
	if manifest.SchemaVersion != SchemaVersion {
		return loadedBundle{}, fmt.Errorf("unsupported bundle schema %q", manifest.SchemaVersion)
	}
	objects, err := decodeJSONL[ObjectRecord](parts["objects.jsonl"])
	if err != nil {
		return loadedBundle{}, fmt.Errorf("objects.jsonl: %w", err)
	}
	evidence, err := decodeJSONL[EvidenceRecord](parts["evidence.jsonl"])
	if err != nil {
		return loadedBundle{}, fmt.Errorf("evidence.jsonl: %w", err)
	}
	blobs := map[string][]byte{}
	for name, raw := range parts {
		if !strings.HasPrefix(name, "blobs/sha256/") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimPrefix(name, "blobs/sha256/"), ".json")
		if len(base) != 64 || digest(raw) != base {
			return loadedBundle{}, fmt.Errorf("blob checksum mismatch %s", name)
		}
		blobs[base] = raw
	}
	objectsRaw, _ := jsonLines(objects)
	evidenceRaw, _ := jsonLines(evidence)
	if bundleDigest(objectsRaw, evidenceRaw, blobs) != manifest.BundleHash {
		return loadedBundle{}, errors.New("bundle hash mismatch")
	}
	if manifest.ObjectCount != len(objects) || manifest.EvidenceCount != len(evidence) {
		return loadedBundle{}, errors.New("bundle manifest counts do not match contents")
	}
	return loadedBundle{Manifest: manifest, Objects: objects, Evidence: evidence, Blobs: blobs}, nil
}
func scanInjection(subject string, raw []byte, executable bool) []Finding {
	lower := strings.ToLower(string(raw))
	patterns := []string{"ignore previous instructions", "ignore all previous", "override system", "developer message", "<system>", "do not obey", "jailbreak", "bypass owner review", "disable safety"}
	out := []Finding{}
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			severity := "warning"
			if executable {
				severity = "blocked"
			}
			out = append(out, Finding{Severity: severity, Code: "instruction_injection_signal", Subject: subject, Detail: "matched instruction-like control phrase: " + pattern})
			break
		}
	}
	remote := regexp.MustCompile(`(?i)\b(?:https?|file)://|(?:\.\./){2,}`)
	if remote.Match(raw) {
		severity := "warning"
		if executable {
			severity = "blocked"
		}
		out = append(out, Finding{Severity: severity, Code: "remote_or_path_reference", Subject: subject, Detail: "payload contains remote/file/path reference; executable memory imports fail closed"})
	}
	return out
}

func executableType(typeID string) bool {
	value := strings.ToLower(typeID)
	for _, token := range []string{"agent-asset", "prompt", "skill", "tool_recipe", "mcp", "procedure", "procedural", "constraint", "rule"} {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func validateBundle(bundle loadedBundle) ([]Finding, error) {
	findings := []Finding{}
	seenObjects := map[string]bool{}
	for _, object := range bundle.Objects {
		if object.ObjectID == "" || object.TypeID == "" || object.CurrentRevision <= 0 {
			return nil, errors.New("bundle contains invalid object identity")
		}
		if seenObjects[object.ObjectID] {
			return nil, fmt.Errorf("duplicate object %s", object.ObjectID)
		}
		seenObjects[object.ObjectID] = true
		currentFound := false
		for _, revision := range object.Revisions {
			blob, ok := bundle.Blobs[revision.BlobSHA256]
			if !ok {
				return nil, fmt.Errorf("object %s R%d blob missing", object.ObjectID, revision.Revision)
			}
			if !json.Valid(blob) {
				return nil, fmt.Errorf("object %s R%d payload is not JSON", object.ObjectID, revision.Revision)
			}
			if contracts.HashBytes(blob) != revision.ContentHash {
				return nil, fmt.Errorf("object %s R%d content hash mismatch", object.ObjectID, revision.Revision)
			}
			if revision.Revision == object.CurrentRevision {
				currentFound = true
			}
			findings = append(findings, scanInjection("object:"+object.ObjectID+"@R"+strconv.Itoa(revision.Revision), blob, executableType(object.TypeID))...)
		}
		if !currentFound {
			return nil, fmt.Errorf("object %s current revision is absent", object.ObjectID)
		}
	}
	seenEvidence := map[string]bool{}
	for _, item := range bundle.Evidence {
		if item.EvidenceID == "" || item.BlobSHA256 == "" {
			return nil, errors.New("bundle contains invalid Evidence identity")
		}
		if seenEvidence[item.EvidenceID] {
			return nil, fmt.Errorf("duplicate Evidence %s", item.EvidenceID)
		}
		seenEvidence[item.EvidenceID] = true
		blob, ok := bundle.Blobs[item.BlobSHA256]
		if !ok {
			return nil, fmt.Errorf("Evidence %s blob missing", item.EvidenceID)
		}
		envelope, canonical, err := contracts.ParseEvidence(blob)
		if err != nil {
			return nil, fmt.Errorf("Evidence %s schema: %w", item.EvidenceID, err)
		}
		if envelope.EvidenceID != item.EvidenceID || contracts.HashBytes(canonical) != item.LineHash {
			return nil, fmt.Errorf("Evidence %s identity/hash mismatch", item.EvidenceID)
		}
		findings = append(findings, scanInjection("evidence:"+item.EvidenceID, canonical, false)...)
	}
	for _, object := range bundle.Objects {
		for _, revision := range object.Revisions {
			for _, sourceID := range revision.SourceEvidenceIDs {
				if !seenEvidence[sourceID] {
					findings = append(findings, Finding{Severity: "warning", Code: "source_evidence_not_selected", Subject: object.ObjectID, Detail: sourceID})
				}
			}
			for _, sourceID := range revision.SourceObjectIDs {
				if !seenObjects[sourceID] {
					findings = append(findings, Finding{Severity: "warning", Code: "source_object_not_selected", Subject: object.ObjectID, Detail: sourceID})
				}
			}
		}
	}
	return findings, nil
}

func (s *Service) Preflight(_ context.Context, path string, options PreflightOptions) (Manifest, CompatibilityReport, error) {
	bundle, err := loadBundle(path)
	if err != nil {
		return Manifest{}, CompatibilityReport{}, err
	}
	findings, err := validateBundle(bundle)
	if err != nil {
		return Manifest{}, CompatibilityReport{}, err
	}
	missing := []string{}
	for _, required := range bundle.Manifest.RequiredCapabilities {
		if !contains(options.Capabilities, required) {
			missing = append(missing, required)
		}
	}
	unmapped := []string{}
	for _, object := range bundle.Objects {
		if len(options.KnownObjectTypes) > 0 && !contains(options.KnownObjectTypes, object.TypeID) {
			unmapped = append(unmapped, object.TypeID)
		}
	}
	missing, unmapped = unique(missing), unique(unmapped)
	blocked := false
	for _, finding := range findings {
		if finding.Severity == "blocked" || finding.Severity == "error" {
			blocked = true
		}
	}
	degradations := []string{}
	presentationFallback := false
	if len(unmapped) > 0 {
		if options.SupportsPresentations {
			presentationFallback = true
			degradations = append(degradations, "unknown object types are presentation-only until explicitly mapped")
		} else {
			degradations = append(degradations, "unknown object types remain protected generic candidates")
		}
	}
	if len(missing) > 0 {
		degradations = append(degradations, "target lacks source capabilities; behavior is not portable without degradation")
	}
	report := CompatibilityReport{Compatible: !blocked && len(missing) == 0 && len(unmapped) == 0, Blocked: blocked, TargetID: strings.TrimSpace(options.TargetID), MissingCapabilities: missing, UnmappedObjectTypes: unmapped, Findings: findings, Degradations: degradations, PermissionDelta: []string{}, PresentationFallback: presentationFallback, ImportMode: "evidence_quarantine+protected_candidate_only"}
	return bundle.Manifest, report, nil
}
func (s *Service) Import(ctx context.Context, path string, options ImportOptions) (ImportResult, error) {
	options.TargetProjectID = strings.TrimSpace(options.TargetProjectID)
	options.IdempotencyKey = strings.TrimSpace(options.IdempotencyKey)
	if options.TargetProjectID == "" || options.IdempotencyKey == "" {
		return ImportResult{}, errors.New("target_project_id and idempotency_key are required")
	}
	var exists int
	if err := s.control.DB.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE project_id=?`, options.TargetProjectID).Scan(&exists); err != nil {
		return ImportResult{}, err
	}
	if exists != 1 {
		return ImportResult{}, errors.New("target project does not exist")
	}
	manifest, report, err := s.Preflight(ctx, path, PreflightOptions{TargetID: options.TargetID, Capabilities: options.Capabilities, KnownObjectTypes: options.KnownObjectTypes, SupportsPresentations: options.SupportsPresentations})
	if err != nil {
		return ImportResult{}, err
	}
	if report.Blocked {
		return ImportResult{}, errors.New("portable bundle preflight is blocked; no import writes were performed")
	}
	bundle, err := loadBundle(path)
	if err != nil {
		return ImportResult{}, err
	}
	result := ImportResult{BundleID: manifest.BundleID, TargetProjectID: options.TargetProjectID, Compatibility: report, CandidateObjectIDs: []string{}, NoDirectActivation: true}
	for _, item := range bundle.Evidence {
		appendResult, err := s.ledger.Append(ctx, bundle.Blobs[item.BlobSHA256])
		if err != nil {
			return ImportResult{}, fmt.Errorf("import Evidence %s: %w", item.EvidenceID, err)
		}
		if appendResult.Duplicate {
			result.EvidenceDuplicates++
		} else {
			result.EvidenceImported++
		}
		if !appendResult.Duplicate {
			// Ledger.Append creates an Inbox fallback projection for newly imported
			// Evidence. Upgrade only that fallback into an explicit quarantine marker.
			// A pre-existing local Evidence keeps its existing project relationships.
			_, err = s.control.DB.ExecContext(ctx, `INSERT INTO record_projects(record_type,record_id,project_id,relation,is_primary,created_at) VALUES('evidence',?,?, 'bundle_quarantine',1,?) ON CONFLICT(record_type,record_id,project_id) DO UPDATE SET relation=CASE WHEN record_projects.relation='fallback' THEN 'bundle_quarantine' ELSE record_projects.relation END,is_primary=CASE WHEN record_projects.relation='fallback' THEN 1 ELSE record_projects.is_primary END`, item.EvidenceID, portfolio.InboxProjectID, time.Now().UTC().Format(time.RFC3339Nano))
			if err != nil {
				return ImportResult{}, fmt.Errorf("quarantine Evidence %s: %w", item.EvidenceID, err)
			}
		}
	}
	for _, record := range bundle.Objects {
		revisionPayloads := map[string]json.RawMessage{}
		total := 0
		for _, revision := range record.Revisions {
			blob := bundle.Blobs[revision.BlobSHA256]
			total += len(blob)
			revisionPayloads[strconv.Itoa(revision.Revision)] = json.RawMessage(append([]byte(nil), blob...))
		}
		if total > 1536<<10 {
			return ImportResult{}, fmt.Errorf("object %s revision payloads exceed 1.5 MiB candidate import bound", record.ObjectID)
		}
		candidate := ImportCandidate{BundleID: manifest.BundleID, OriginalProjectID: record.ProjectID, OriginalObject: record, RevisionPayloads: revisionPayloads, Compatibility: report, ImportedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		raw, err := json.Marshal(candidate)
		if err != nil {
			return ImportResult{}, err
		}
		candidateID := "portable-candidate-" + contracts.HashBytes([]byte(manifest.BundleID + "\x00" + options.TargetProjectID + "\x00" + record.ObjectID))[:24]
		actualEvidence := []string{}
		selectedEvidence := map[string]bool{}
		for _, item := range bundle.Evidence {
			selectedEvidence[item.EvidenceID] = true
		}
		for _, sourceID := range evidenceIDs(record) {
			if selectedEvidence[sourceID] {
				actualEvidence = append(actualEvidence, sourceID)
			}
		}
		object, err := s.harness.Materialize(ctx, harness.MaterializeInput{
			ObjectID: candidateID, TypeID: ImportCandidateTypeV1, ProjectID: options.TargetProjectID, Status: "candidate",
			Payload: raw, Confidence: 1, Importance: .65, SourceEvidenceIDs: unique(actualEvidence), SourceObjectIDs: []string{},
			PluginID: PluginID, PluginVersion: PluginVersion,
			IdempotencyKey: "portable-import:" + manifest.BundleID + ":" + options.TargetProjectID + ":" + record.ObjectID,
		})
		if err != nil {
			return ImportResult{}, fmt.Errorf("materialize portable candidate %s: %w", record.ObjectID, err)
		}
		result.CandidateObjectIDs = append(result.CandidateObjectIDs, object.ObjectID)
	}
	result.CandidateObjectIDs = unique(result.CandidateObjectIDs)
	return result, nil
}
func Inspect(path string) (Manifest, []ObjectRecord, []EvidenceRecord, error) {
	bundle, err := loadBundle(path)
	if err != nil {
		return Manifest{}, nil, nil, err
	}
	if _, err := validateBundle(bundle); err != nil {
		return Manifest{}, nil, nil, err
	}
	return bundle.Manifest, bundle.Objects, bundle.Evidence, nil
}
