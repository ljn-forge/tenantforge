-- +goose Up
ALTER TABLE departments ADD COLUMN created_by uuid REFERENCES users(id);
ALTER TABLE roles ADD COLUMN created_by uuid REFERENCES users(id);
ALTER TABLE invitations ADD COLUMN created_by uuid REFERENCES users(id);
