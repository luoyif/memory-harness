package localembedding

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

const (
	Algorithm         = "local-feature-hash-v1"
	Dimensions        = 384
	MinimumSimilarity = 0.30
	maxRunes          = 32000
)

// Encode creates a deterministic, dependency-free embedding from word and
// character n-gram features. It is intentionally a rebuildable local
// projection: callers must not describe it as a hosted semantic model.
func Encode(text string) []float32 {
	vector := make([]float32, Dimensions)
	runes := []rune(strings.ToLower(strings.TrimSpace(text)))
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	normalized := make([]rune, 0, len(runes))
	for _, r := range runes {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			normalized = append(normalized, r)
		} else {
			normalized = append(normalized, ' ')
		}
	}
	for _, field := range strings.Fields(string(normalized)) {
		add(vector, "w:"+field, 2)
		chars := []rune(field)
		for n := 3; n <= 5 && n <= len(chars); n++ {
			for i := 0; i+n <= len(chars); i++ {
				add(vector, "c:"+string(chars[i:i+n]), 1)
			}
		}
	}
	compact := make([]rune, 0, len(normalized))
	for _, r := range normalized {
		if r != ' ' {
			compact = append(compact, r)
		}
	}
	for n := 2; n <= 3 && n <= len(compact); n++ {
		for i := 0; i+n <= len(compact); i++ {
			window := compact[i : i+n]
			hasHan := false
			for _, r := range window {
				if unicode.Is(unicode.Han, r) {
					hasHan = true
					break
				}
			}
			if hasHan {
				add(vector, "h:"+string(window), 1.5)
			}
		}
	}
	norm := float64(0)
	for _, value := range vector {
		norm += float64(value * value)
	}
	if norm == 0 {
		return vector
	}
	scale := float32(1 / math.Sqrt(norm))
	for index := range vector {
		vector[index] *= scale
	}
	return vector
}

func add(vector []float32, feature string, weight float32) {
	h := fnv.New64a()
	_, _ = h.Write([]byte(feature))
	sum := h.Sum64()
	index := int(sum % uint64(len(vector)))
	if sum&(uint64(1)<<63) != 0 {
		weight = -weight
	}
	vector[index] += weight
}

func Marshal(vector []float32) []byte {
	raw := make([]byte, len(vector)*4)
	for index, value := range vector {
		binary.LittleEndian.PutUint32(raw[index*4:], math.Float32bits(value))
	}
	return raw
}

func Unmarshal(raw []byte, dimensions int) ([]float32, error) {
	if dimensions <= 0 || len(raw) != dimensions*4 {
		return nil, errors.New("invalid local embedding payload")
	}
	vector := make([]float32, dimensions)
	for index := range vector {
		vector[index] = math.Float32frombits(binary.LittleEndian.Uint32(raw[index*4:]))
	}
	return vector, nil
}

func Similarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	dot, leftNorm, rightNorm := float64(0), float64(0), float64(0)
	for index := range left {
		l, r := float64(left[index]), float64(right[index])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / math.Sqrt(leftNorm*rightNorm)
}
