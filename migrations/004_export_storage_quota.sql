-- +goose Up
ALTER TABLE jobs ADD COLUMN result_bytes bigint NOT NULL DEFAULT 0 CHECK(result_bytes>=0);
