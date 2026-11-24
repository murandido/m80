-- A Tropa no longer has an agency of its own. Which agency an issuance belongs to depends on
-- who issued it (one workbook mixes issuers of different agencies), so the agency moved into
-- each issuer mapping (mappings.issuers[].agency_id).
--
-- Destructive: the column's values are dropped. They are not copied into the issuer mappings
-- on purpose - the one agency of the Tropa is exactly the wrong answer for some of its
-- issuers, and copying it would import their rows into it silently. An issuer without an
-- agency makes its rows "needs mapping" until somebody picks one, which is visible.
ALTER TABLE injunction_groups DROP COLUMN agency_id;
