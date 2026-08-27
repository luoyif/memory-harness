/// <reference types="vite/client" />

type DesktopBootstrap = {
  endpoint: string
  session_id: string
  token: string
  csrf_token: string
  expires_at: string
  version: string
}

declare global {
  interface Window {
    go?: {
      main?: {
        DesktopBridge?: {
          Bootstrap: () => Promise<DesktopBootstrap>
          InstallPluginPackage?: (projectID: string, capabilities: string[], developerMode: boolean) => Promise<{ plugin_id: string; version: string }>
          ExportBackup?: () => Promise<string>
          ExportPortableBundle?: (projectID: string, objectIDs: string[], includeDependencies: boolean) => Promise<{
            path: string
            manifest: { bundle_id: string; bundle_hash: string; object_count: number; evidence_count: number; required_capabilities: string[]; signature: { status: string; algorithm: string } }
          }>
          PreflightPortableBundle?: () => Promise<{
            path: string
            manifest: { bundle_id: string; bundle_hash: string; source_project_id: string; object_count: number; evidence_count: number; required_capabilities: string[]; signature: { status: string; algorithm: string } }
            report: { compatible: boolean; blocked: boolean; missing_capabilities: string[]; unmapped_object_types: string[]; findings: Array<{ severity: string; code: string; subject: string; detail: string }>; degradations: string[]; permission_delta: string[]; presentation_fallback: boolean; import_mode: string }
          }>
          ImportPortableBundle?: (projectID: string, path: string, expectedBundleID: string, expectedBundleHash: string, idempotencyKey: string) => Promise<{
            bundle_id: string; target_project_id: string; evidence_imported: number; evidence_duplicates: number; candidate_object_ids: string[]; no_direct_activation: boolean
          }>
          ProbeDeepSeekHarness?: () => Promise<{
            status: string
            adapter_version: string
            plugin_path: string
            plugin_name: string
            plugin_version: string
            contract_verified: boolean
            runtime_reachable: boolean
            runtime_endpoint: string
            checks: Array<{ name: string; status: string; detail: string }>
            limits: string[]
          }>
        }
      }
    }
  }
}

export {}
