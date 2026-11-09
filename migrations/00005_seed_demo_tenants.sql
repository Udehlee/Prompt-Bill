-- +goose Up
-- Seed 3 Demo Tenants
INSERT INTO tenants (id, name, created_at)
VALUES
  ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', 'Acme AI Labs', NOW() - INTERVAL '10 days'),
  ('40e9e061-7b12-41cd-bf64-4e7dc88fc741', 'FinTech Global', NOW() - INTERVAL '10 days'),
  ('22ef6ab9-de67-4f90-9c84-53b03303ef15', 'HealthTech Systems', NOW() - INTERVAL '10 days')
ON CONFLICT (id) DO NOTHING;

-- Seed Demo Keys (Plaintext keys documented in README)
INSERT INTO api_keys (id, tenant_id, name, key_prefix, key_hash, revoked_at, created_at)
VALUES
  ('23b4fa30-6d13-4e0b-b0b4-490e5c5cf6f0', 'b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', 'Demo Key 1', 'trk_live_af6b5e01', 'f30596787d14ca177a5f43bd3a8ec00f1fd075a40d7f7ff27c2f82f2284a48da', NULL, NOW() - INTERVAL '10 days'),
  ('8f15d1c1-2846-4b1d-b59d-69261b6c8ba5', '40e9e061-7b12-41cd-bf64-4e7dc88fc741', 'Demo Key 2', 'trk_live_91ebe5be', '99fe42ba17e19315b5476951c7a1079b8b55f7601f7ecadbd8a5e5a257b5a65e', NULL, NOW() - INTERVAL '10 days'),
  ('3241affd-5c38-4495-81ba-66cc6d9ff4ea', '22ef6ab9-de67-4f90-9c84-53b03303ef15', 'Demo Key 3', 'trk_live_0ba44580', '582efefaf474608708d4dbd15524aa6966b6f844b5853dbf7ef77c06a02c690a', NULL, NOW() - INTERVAL '10 days')
ON CONFLICT (id) DO NOTHING;

-- Seed Model Rates for Groq / OpenAI
INSERT INTO pricing_rates (version, provider, model, prompt_rate_per_million, completion_rate_per_million, cached_prompt_rate_per_million, effective_from)
VALUES
  ('2026-11-groq', 'groq', 'openai/gpt-oss-20b', 75000, 300000, 37500, '2026-01-01 00:00:00+00'),
  ('2026-11-groq', 'groq', 'openai/gpt-oss-120b', 150000, 600000, 75000, '2026-01-01 00:00:00+00'),
  ('2026-11-groq', 'groq', 'qwen/qwen3.8-27b', 800000, 4000000, 400000, '2026-01-01 00:00:00+00')
ON CONFLICT (version, provider, model) DO NOTHING;

-- Seed Sample Usage Requests for Demo Tenants
INSERT INTO usage_requests (tenant_id, api_key_id, user_id, feature_name, provider, endpoint, requested_model, reported_model, status_code, latency_ms, input_tokens, output_tokens, total_tokens, usage_status, estimated_cost_microdollars, pricing_version, is_streaming, created_at)
VALUES
  -- Acme AI Labs
  ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', '23b4fa30-6d13-4e0b-b0b4-490e5c5cf6f0', 'alice', 'customer_support', 'groq', '/v1/chat/completions', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b', 200, 342, 145, 230, 375, 'complete', 80, '2026-11-groq', false, NOW() - INTERVAL '15 minutes'),
  ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', '23b4fa30-6d13-4e0b-b0b4-490e5c5cf6f0', 'bob', 'shopping_assistant', 'groq', '/v1/chat/completions', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b', 200, 480, 210, 412, 622, 'complete', 139, '2026-11-groq', true, NOW() - INTERVAL '45 minutes'),
  ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', '23b4fa30-6d13-4e0b-b0b4-490e5c5cf6f0', 'alice', 'customer_support', 'groq', '/v1/chat/completions', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b', 200, 290, 95, 180, 275, 'complete', 61, '2026-11-groq', false, NOW() - INTERVAL '2 hours'),
  ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', '23b4fa30-6d13-4e0b-b0b4-490e5c5cf6f0', 'charlie', 'product_recommendations', 'groq', '/v1/chat/completions', 'openai/gpt-oss-120b', 'openai/gpt-oss-120b', 200, 610, 350, 520, 870, 'complete', 364, '2026-11-groq', false, NOW() - INTERVAL '5 hours'),
  -- FinTech Global
  ('40e9e061-7b12-41cd-bf64-4e7dc88fc741', '8f15d1c1-2846-4b1d-b59d-69261b6c8ba5', 'marcus', 'fraud_detection', 'groq', '/v1/chat/completions', 'qwen/qwen3.8-27b', 'qwen/qwen3.8-27b', 200, 520, 850, 310, 1160, 'complete', 1920, '2026-11-groq', false, NOW() - INTERVAL '10 minutes'),
  ('40e9e061-7b12-41cd-bf64-4e7dc88fc741', '8f15d1c1-2846-4b1d-b59d-69261b6c8ba5', 'elena', 'portfolio_analyzer', 'groq', '/v1/chat/completions', 'openai/gpt-oss-120b', 'openai/gpt-oss-120b', 200, 780, 620, 950, 1570, 'complete', 663, '2026-11-groq', true, NOW() - INTERVAL '30 minutes'),
  ('40e9e061-7b12-41cd-bf64-4e7dc88fc741', '8f15d1c1-2846-4b1d-b59d-69261b6c8ba5', 'marcus', 'fraud_detection', 'groq', '/v1/chat/completions', 'qwen/qwen3.8-27b', 'qwen/qwen3.8-27b', 200, 490, 720, 280, 1000, 'complete', 1696, '2026-11-groq', false, NOW() - INTERVAL '1 hour'),
  ('40e9e061-7b12-41cd-bf64-4e7dc88fc741', '8f15d1c1-2846-4b1d-b59d-69261b6c8ba5', 'david', 'risk_compliance', 'groq', '/v1/chat/completions', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b', 200, 310, 400, 350, 750, 'complete', 135, '2026-11-groq', false, NOW() - INTERVAL '3 hours'),
  -- HealthTech Systems
  ('22ef6ab9-de67-4f90-9c84-53b03303ef15', '3241affd-5c38-4495-81ba-66cc6d9ff4ea', 'dr_sarah', 'clinical_triage', 'groq', '/v1/chat/completions', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b', 200, 280, 520, 340, 860, 'complete', 141, '2026-11-groq', false, NOW() - INTERVAL '20 minutes'),
  ('22ef6ab9-de67-4f90-9c84-53b03303ef15', '3241affd-5c38-4495-81ba-66cc6d9ff4ea', 'dr_chen', 'doctor_copilot', 'groq', '/v1/chat/completions', 'openai/gpt-oss-120b', 'openai/gpt-oss-120b', 200, 650, 410, 620, 1030, 'complete', 433, '2026-11-groq', true, NOW() - INTERVAL '1 hour'),
  ('22ef6ab9-de67-4f90-9c84-53b03303ef15', '3241affd-5c38-4495-81ba-66cc6d9ff4ea', 'nurse_emma', 'patient_intake', 'groq', '/v1/chat/completions', 'openai/gpt-oss-20b', 'openai/gpt-oss-20b', 200, 330, 260, 180, 440, 'complete', 74, '2026-11-groq', false, NOW() - INTERVAL '4 hours')
ON CONFLICT (id) DO NOTHING;

-- +goose Down
DELETE FROM usage_requests WHERE tenant_id IN ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', '40e9e061-7b12-41cd-bf64-4e7dc88fc741', '22ef6ab9-de67-4f90-9c84-53b03303ef15');
DELETE FROM api_keys WHERE id IN ('23b4fa30-6d13-4e0b-b0b4-490e5c5cf6f0', '8f15d1c1-2846-4b1d-b59d-69261b6c8ba5', '3241affd-5c38-4495-81ba-66cc6d9ff4ea');
DELETE FROM tenants WHERE id IN ('b3363179-3f94-43e5-96f1-f9d9cb4bb9cd', '40e9e061-7b12-41cd-bf64-4e7dc88fc741', '22ef6ab9-de67-4f90-9c84-53b03303ef15');
