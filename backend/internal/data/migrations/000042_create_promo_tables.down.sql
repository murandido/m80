-- Safe to run: nothing outside these three tables references them, and the feature they
-- support has no fallback path in the older API - it simply is not there. Dropping them
-- does lose the enrolment history for good, so take a dump first if the data matters
-- (make db-backup).
DROP TABLE IF EXISTS promo_enrollments;
DROP TABLE IF EXISTS loyalty_documents;
DROP TABLE IF EXISTS promo_recipes;
