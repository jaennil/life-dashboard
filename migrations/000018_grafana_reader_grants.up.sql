-- Grant the Grafana datasource role read access to everything in the schema.
--
-- GRANT ... ON ALL TABLES only covers tables that exist right now, which is why
-- every new migration used to break Grafana with "permission denied for table".
-- ALTER DEFAULT PRIVILEGES is the part that fixes it permanently: it applies to
-- tables created later. Default privileges are keyed to the creating role, so it
-- must name the role that runs the migrations - current_user - rather than a
-- hardcoded owner.
--
-- Guarded by a role check so local, CI and fresh databases, where grafana_reader
-- does not exist, run this as a no-op instead of failing the whole boot.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'grafana_reader') THEN
        EXECUTE 'GRANT USAGE ON SCHEMA public TO grafana_reader';
        EXECUTE 'GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana_reader';
        EXECUTE format(
            'ALTER DEFAULT PRIVILEGES FOR ROLE %I IN SCHEMA public GRANT SELECT ON TABLES TO grafana_reader',
            current_user
        );
    END IF;
END
$$;
