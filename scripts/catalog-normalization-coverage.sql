-- Read-only coverage of the active snapshot; this is not a relevance/accuracy metric.
WITH active AS MATERIALIZED (
    SELECT id,regexp_replace(schema_version,'^.*/','') AS policy_version
    FROM catalog_snapshots WHERE id=(SELECT snapshot_id FROM catalog_active)
), chars AS MATERIALIZED (
    SELECT payload->'Characteristics' AS c
    FROM catalog_records JOIN active ON snapshot_id=active.id
    WHERE kind='offer'
), feature_counts AS (
    SELECT key AS field,value->>'Status' AS status,count(*) AS n
    FROM chars CROSS JOIN LATERAL jsonb_each(c)
    WHERE key<>'PolicyVersion'
    GROUP BY key,value->>'Status'
), feature_objects AS (
    SELECT field,jsonb_object_agg(status,n) AS counts
    FROM feature_counts GROUP BY field
), roles AS (
    SELECT coalesce(nullif(c#>>'{OfferKind,Value}',''),'unknown') AS role,count(*) AS n
    FROM chars GROUP BY 1
)
SELECT jsonb_build_object(
    'policy_version',(SELECT policy_version FROM active),
    'offers',(SELECT count(*) FROM chars),
    'with_snapshot_policy',(SELECT count(*) FROM chars WHERE c->>'PolicyVersion'=(SELECT policy_version FROM active)),
    'coverage',(SELECT jsonb_object_agg(field,counts) FROM feature_objects),
    'roles',(SELECT jsonb_object_agg(role,n) FROM roles)
);
