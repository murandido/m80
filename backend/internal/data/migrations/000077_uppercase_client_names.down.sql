-- Intentionally a no-op: the original spelling of each name is not kept anywhere, so there is
-- nothing to restore it from. Upper-case names are valid under the previous code as well, so
-- rolling the API back needs no schema change. The pre-deploy dump is the way back if the
-- old spellings are ever needed.
SELECT 1;
