-- Runs once, when the PostgreSQL volume is first initialised: creates a separate database for
-- the integration tests so they never touch the app's data.
CREATE DATABASE yozigo_test;
