package adaptation

const ChangeProposalSchemaV1 = `{
  "type":"object",
  "required":["proposal_id","project_id","source_case_ids","source_pattern_ids","source_run_ids","source_outcome_run_ids","base_blueprint_id","base_blueprint_version","base_blueprint_hash","patch","effective_blueprint_hash","predicted_fix","predicted_regressions","evaluation_suite","minimum_sample","stop_conditions","permission_impact","privacy_impact","cost_impact","proposer_id","evaluation_object_ids","canary_scope","overlay_ttl_seconds","rollback_blueprint_hash","created_at"],
  "properties":{
    "proposal_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},
    "source_case_ids":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","maxLength":240}},
    "source_pattern_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},
    "source_run_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "source_outcome_run_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "base_blueprint_id":{"type":"string","maxLength":240},"base_blueprint_version":{"type":"string","maxLength":80},"base_blueprint_hash":{"type":"string","maxLength":160},
    "patch":{"type":"object","required":["role","config"],"properties":{"role":{"type":"string","maxLength":120},"config":{"type":"object"}},"additionalProperties":false},
    "effective_blueprint_hash":{"type":"string","maxLength":160},"predicted_fix":{"type":"string","maxLength":8000},
    "predicted_regressions":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":2000}},
    "evaluation_suite":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","maxLength":240}},"minimum_sample":{"type":"integer","minimum":1,"maximum":10000},
    "stop_conditions":{"type":"object","required":["max_regression_rate","stop_on_safety_failure"],"properties":{"max_regression_rate":{"type":"number","minimum":0,"maximum":1},"stop_on_safety_failure":{"type":"boolean"}},"additionalProperties":false},
    "permission_impact":{"type":"array","maxItems":20,"items":{"type":"string","maxLength":160}},
    "privacy_impact":{"type":"string","maxLength":4000},"cost_impact":{"type":"string","maxLength":4000},
    "proposer_id":{"type":"string","maxLength":200},"verifier_id":{"type":"string","maxLength":200},
    "evaluation_object_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "canary_scope":{"type":"string","maxLength":240},"overlay_ttl_seconds":{"type":"integer","minimum":60,"maximum":86400},
    "rollback_blueprint_hash":{"type":"string","maxLength":160},"created_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`

const CaseOverlaySchemaV1 = `{
  "type":"object",
  "required":["overlay_id","project_id","proposal_id","base_blueprint_id","base_blueprint_version","base_blueprint_hash","effective_blueprint_hash","patch","effective_blueprint","permission_delta","ttl_seconds","created_at","expires_at"],
  "properties":{
    "overlay_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},"proposal_id":{"type":"string","maxLength":240},
    "base_blueprint_id":{"type":"string","maxLength":240},"base_blueprint_version":{"type":"string","maxLength":80},"base_blueprint_hash":{"type":"string","maxLength":160},"effective_blueprint_hash":{"type":"string","maxLength":160},
    "patch":{"type":"object","required":["role","config"],"properties":{"role":{"type":"string","maxLength":120},"config":{"type":"object"}},"additionalProperties":false},
    "effective_blueprint":{"type":"object"},"permission_delta":{"type":"array","maxItems":20,"items":{"type":"string","maxLength":160}},
    "ttl_seconds":{"type":"integer","minimum":60,"maximum":86400},"created_at":{"type":"string","maxLength":80},"expires_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`
