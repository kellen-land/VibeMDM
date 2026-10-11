-- Features the agent reported at its last check-in, so the server never hands a policy to an
-- agent that would silently skip part of it.
ALTER TABLE computers ADD COLUMN agent_capabilities JSONB NOT NULL DEFAULT '[]'::jsonb;
