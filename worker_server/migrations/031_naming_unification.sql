-- 031: Naming Unification
-- Rename contact column title -> job_title to match domain model (Contact.JobTitle)

ALTER TABLE contacts RENAME COLUMN title TO job_title;
