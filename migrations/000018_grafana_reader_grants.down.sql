-- Revokes what the up migration granted. USAGE ON SCHEMA public is deliberately
-- left in place: it may predate this migration and revoking it would break other
-- consumers of the role.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'grafana_reader') THEN
        EXECUTE format(
            'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public REVOKE SELECT ON TABLES FROM grafana_reader',
            current_user
        );
        EXECUTE 'REVOKE SELECT ON ALL TABLES IN SCHEMA public FROM grafana_reader';
    END IF;
END
$$;
