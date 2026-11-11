-- Schema compatibility. compat is the oldest writer level the
-- database still works with. A binary opens a database with a newer
-- schema_version as long as compat is not above its own level, so the
-- additive migrations a newer tock applies never stop an older trackd.
-- A migration that breaks older writers raises compat, carries the
-- breaks-older-writers marker and says so in CHANGELOG.md.
CREATE TABLE schema_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
INSERT INTO schema_meta(key, value) VALUES('compat', '1');
