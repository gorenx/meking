-- name: GetAnalysisCacheSchemaVersion :one
SELECT version
FROM analysis_cache_schema
WHERE id = 1;

-- name: ValidateAnalysisCacheSchema :one
SELECT
    (SELECT
        count(cache_key) +
        count(capability) +
        count(payload) +
        count(created_at)
     FROM analysis_cache
     WHERE 0) +
    (SELECT count(id) + count(version)
     FROM analysis_cache_schema
     WHERE 0) AS valid;

-- name: GetAnalysisCacheEntry :one
SELECT capability, payload, created_at
FROM analysis_cache
WHERE cache_key = ?;

-- name: PutAnalysisCacheEntry :exec
INSERT INTO analysis_cache (
    cache_key,
    capability,
    payload,
    created_at
) VALUES (?, ?, ?, ?)
ON CONFLICT(cache_key) DO UPDATE SET
    capability = excluded.capability,
    payload = excluded.payload,
    created_at = excluded.created_at;
