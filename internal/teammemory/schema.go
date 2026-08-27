package teammemory

const TaskSchemaV1 = `{
  "type":"object",
  "required":["task_id","project_id","title","member_agent_ids","status","created_at","expires_at"],
  "properties":{
    "task_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},
    "title":{"type":"string","minLength":1,"maxLength":500},
    "member_agent_ids":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","maxLength":200}},
    "status":{"type":"string","enum":["active","closed"]},
    "created_at":{"type":"string","maxLength":80},"expires_at":{"type":"string","maxLength":80},"closed_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`

const contributionMetaSchema = `"meta":{"type":"object","required":["agent_id","source_evidence_ids","confidence","epistemic_status","created_at","expires_at"],"properties":{"agent_id":{"type":"string","maxLength":200},"run_id":{"type":"string","maxLength":240},"source_evidence_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},"confidence":{"type":"number","minimum":0,"maximum":1},"epistemic_status":{"type":"string","enum":["observed","inferred","hypothesis","disputed"]},"created_at":{"type":"string","maxLength":80},"expires_at":{"type":"string","maxLength":80}},"additionalProperties":false}`

const PrivateScratchSchemaV1 = `{
  "type":"object","required":["entry_id","task_id","project_id","content","meta"],
  "properties":{"entry_id":{"type":"string","maxLength":200},"task_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},"content":{"type":"string","minLength":1,"maxLength":32000},` + contributionMetaSchema + `},
  "additionalProperties":false
}`

const BlackboardEntrySchemaV1 = `{
  "type":"object","required":["entry_id","task_id","project_id","topic","claim_key","claim_value","content","direct_share_agent_ids","meta"],
  "properties":{
    "entry_id":{"type":"string","maxLength":200},"task_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},
    "topic":{"type":"string","minLength":1,"maxLength":500},"claim_key":{"type":"string","minLength":1,"maxLength":500},"claim_value":{"type":"string","minLength":1,"maxLength":4000},
    "content":{"type":"string","minLength":1,"maxLength":32000},"direct_share_agent_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":200}},` + contributionMetaSchema + `
  },"additionalProperties":false
}`

const ConflictSchemaV1 = `{
  "type":"object","required":["conflict_id","task_id","project_id","topic","claim_key","entry_ids","agent_ids","status","created_at"],
  "properties":{"conflict_id":{"type":"string","maxLength":200},"task_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},"topic":{"type":"string","maxLength":500},"claim_key":{"type":"string","maxLength":500},"entry_ids":{"type":"array","minItems":2,"maxItems":100,"items":{"type":"string","maxLength":200}},"agent_ids":{"type":"array","minItems":2,"maxItems":100,"items":{"type":"string","maxLength":200}},"status":{"type":"string","enum":["needs_review","resolved"]},"created_at":{"type":"string","maxLength":80}},
  "additionalProperties":false
}`

const ProjectDurableSchemaV1 = `{
  "type":"object","required":["durable_id","project_id","task_id","entry_ids","title","summary","body","source_agent_ids","source_run_ids","source_evidence_ids","epistemic_status","generation_status","created_at"],
  "properties":{
    "durable_id":{"type":"string","maxLength":200},"project_id":{"type":"string","maxLength":200},"task_id":{"type":"string","maxLength":200},
    "entry_ids":{"type":"array","minItems":1,"maxItems":200,"items":{"type":"string","maxLength":200}},
    "title":{"type":"string","minLength":1,"maxLength":500},"summary":{"type":"string","maxLength":8000},"body":{"type":"string","minLength":1,"maxLength":48000},
    "source_agent_ids":{"type":"array","maxItems":100,"items":{"type":"string","maxLength":200}},
    "source_run_ids":{"type":"array","maxItems":200,"items":{"type":"string","maxLength":240}},
    "source_evidence_ids":{"type":"array","maxItems":500,"items":{"type":"string","maxLength":240}},
    "epistemic_status":{"type":"string","enum":["observed","inferred","hypothesis","disputed","mixed"]},
    "generation_status":{"type":"string","enum":["owner_selected","human_mixed"]},"created_at":{"type":"string","maxLength":80}
  },"additionalProperties":false
}`
