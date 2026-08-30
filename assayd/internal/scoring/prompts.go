package scoring

const groundednessExtractionPrompt = `You extract atomic factual claims from an answer for
grounding verification.
Rules:
- Emit one verifiable fact per claim. Split compound sentences.
- Resolve pronouns and references using the question and answer so every claim stands alone.
- Exclude greetings, hedges, meta-statements, restatements of the question, and subjective
  opinions. Record excluded content with a concise reason; do not emit it as a claim.
- Do not use outside knowledge. Preserve facts exactly as asserted by the answer.
Return only JSON with claims [{id,text}] and excluded [{text,reason}]. Claim IDs and texts must be
non-blank and IDs must be unique.`

const groundednessVerificationPrompt = `Judge every claim against all supplied context chunks
jointly and use the context only. Do not use outside knowledge.
Assign exactly one verdict for every claim ID:
- supported: directly stated or unambiguously entailed by the context;
- contradicted: the context asserts the opposite;
- unsupported: the context is silent or insufficient, which is not grounded.
Cite one or more supplied chunk IDs for every supported or contradicted verdict; never invent chunk
IDs. Return only JSON with verdicts [{id,verdict,supporting_chunk_ids,reason}]. Do not omit,
duplicate, or invent claim IDs.`

const correctnessPrompt = `You are a strict grader comparing a generated answer to a reference
answer for a question. Grade only factual correctness and completeness relative to the reference.
Ignore style, tone, length, and formatting. Do not reward extra content absent from the reference,
and do not use outside knowledge as ground truth.
Procedure:
1. List at least one key fact from the non-empty reference answer.
2. Mark each fact correct, contradicted, or missing in the generated answer.
3. List generated factual claims that contradict the reference.
4. Explain the result concisely, then assign a score from 0 through 1.
Score 1 means every reference fact is correct with no contradictions. Score 0 means the answer is
wrong, contradictory, or covers no reference facts. A key-fact contradiction caps the score at
0.3 regardless of coverage.
Return only JSON with reference_facts [{fact,status}], contradictions, reasoning, and score.`
