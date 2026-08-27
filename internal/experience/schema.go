package experience

import "strings"

const EvaluationSchemaV1 = `{
  "type":"object",
  "required":["evaluation_id","project_id","target_kind","target_id","protocol","evaluator_id","evaluator_version","verdict","dimensions","confidence","sample_size","source_run_ids","source_outcome_run_ids","evaluated_at"],
  "properties":{
    "evaluation_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},
    "target_kind":{"type":"string","enum":["case","pattern"]},"target_id":{"type":"string","maxLength":240},
    "protocol":{"type":"string","maxLength":80},"evaluator_id":{"type":"string","maxLength":200},"evaluator_version":{"type":"string","maxLength":80},
    "verdict":{"type":"string","enum":["pass","fail","unknown"]},
    "dimensions":{"type":"array","maxItems":40,"items":{"type":"object","required":["name","verdict","confidence"],"properties":{"name":{"type":"string","maxLength":120},"verdict":{"type":"string","enum":["pass","fail","unknown"]},"score":{"type":"number"},"confidence":{"type":"number","minimum":0,"maximum":1},"note":{"type":"string","maxLength":4000}},"additionalProperties":false}},
    "expected":{"type":"string","maxLength":8000},"observed":{"type":"string","maxLength":8000},"confidence":{"type":"number","minimum":0,"maximum":1},"sample_size":{"type":"integer","minimum":1},
    "baseline_ref":{"type":"string","maxLength":240},"challenger_ref":{"type":"string","maxLength":240},
    "source_run_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},"source_outcome_run_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "notes":{"type":"string","maxLength":8000},"evaluated_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`

const CaseSchemaV1 = `{
  "type":"object",
  "required":["case_id","project_id","source_run_id","task_features","delivery","outcome_run_ids","outcome_metrics","cost","evaluation_object_ids","result","secondary_failure_dimensions","transfer_scope","sensitivity","source_artifact_refs","source_hash","generated_at"],
  "properties":{
    "case_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},"source_run_id":{"type":"string","maxLength":240},
    "plan_id":{"type":"string","maxLength":240},"receipt_id":{"type":"string","maxLength":240},"request_fingerprint":{"type":"string","maxLength":160},
    "blueprint_id":{"type":"string","maxLength":240},"blueprint_version":{"type":"string","maxLength":80},"blueprint_hash":{"type":"string","maxLength":160},
    "adapter_id":{"type":"string","maxLength":200},"runtime":{"type":"string","maxLength":120},"protocol_version":{"type":"string","maxLength":120},
    "task_features":{"type":"object","maxProperties":40,"additionalProperties":{"type":"string","maxLength":500}},
    "delivery":{"type":"object","required":["total","delivered","trimmed","denied","failed","delivery_unverified"],"properties":{"total":{"type":"integer","minimum":0},"delivered":{"type":"integer","minimum":0},"trimmed":{"type":"integer","minimum":0},"denied":{"type":"integer","minimum":0},"failed":{"type":"integer","minimum":0},"delivery_unverified":{"type":"integer","minimum":0},"evidence_level":{"type":"string","maxLength":80},"completeness":{"type":"string","maxLength":80}},"additionalProperties":false},
    "outcome_run_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "outcome_metrics":{"type":"array","maxItems":200,"items":{"type":"object","required":["name","value","confidence"],"properties":{"name":{"type":"string","maxLength":160},"value":{},"confidence":{"type":"number","minimum":0,"maximum":1}},"additionalProperties":false}},
    "cost":{"type":"object","properties":{"tokens":{"type":"integer","minimum":0},"latency_ms":{"type":"integer","minimum":0},"money_minor":{"type":"integer","minimum":0},"safety_events":{"type":"integer","minimum":0}},"additionalProperties":false},
    "evaluation_object_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},"result":{"type":"string","enum":["pass","fail","unknown"]},
    "primary_failure_dimension":{"type":"string","maxLength":160},"secondary_failure_dimensions":{"type":"array","maxItems":40,"items":{"type":"string","maxLength":160}},
    "diagnosis":{"type":"string","maxLength":8000},"counterfactual_hypothesis":{"type":"string","maxLength":8000},
    "transfer_scope":{"type":"array","maxItems":40,"items":{"type":"string","maxLength":240}},"expires_at":{"type":"string","maxLength":80},
    "sensitivity":{"type":"string","enum":["standard","sensitive"]},"source_artifact_refs":{"type":"array","maxItems":500,"items":{"type":"string","maxLength":300}},
    "source_hash":{"type":"string","maxLength":160},"generated_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`

const PatternSchemaV1 = `{
  "type":"object",
  "required":["pattern_id","project_id","normalized_pattern","supporting_case_ids","counterexample_case_ids","target_components","conditions","expected_effect","confidence","sample_size","evaluation_object_ids","known_regressions","negative_domains","generated_at"],
  "properties":{
    "pattern_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},"normalized_pattern":{"type":"string","maxLength":8000},
    "supporting_case_ids":{"type":"array","minItems":2,"maxItems":200,"items":{"type":"string","maxLength":240}},"counterexample_case_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "target_components":{"type":"array","maxItems":80,"items":{"type":"string","maxLength":240}},"conditions":{"type":"array","maxItems":80,"items":{"type":"string","maxLength":1000}},
    "expected_effect":{"type":"string","maxLength":8000},"confidence":{"type":"number","minimum":0,"maximum":1},"sample_size":{"type":"integer","minimum":2},
    "evaluation_object_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":240}},"known_regressions":{"type":"array","maxItems":80,"items":{"type":"string","maxLength":2000}},
    "negative_domains":{"type":"array","maxItems":80,"items":{"type":"string","maxLength":500}},"last_validated":{"type":"string","maxLength":80},"generated_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`

var EvaluationSchemaV2 = strings.Replace(EvaluationSchemaV1, `"enum":["case","pattern"]`, `"enum":["case","pattern","change_proposal"]`, 1)
