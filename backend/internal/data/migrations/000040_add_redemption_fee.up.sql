-- Azul charges a "Taxa de Resgate" on award tickets issued less than 120 days before the
-- flight: R$ 69,90 domestic and R$ 174,90 international, per passenger and per trecho
-- (so a round trip counts twice). A Diamante Unique account is exempt from all of it,
-- which is the whole reason to record this: choosing which account to issue on now has a
-- number attached, and until today that number lived nowhere.
--
-- Read off azul.com.br/br/pt/programa-fidelidade/taxa-resgate-pontos on 2026-08-19.

-- The rule's parameters live on the program, not in Go, for the same reason the CPF caps
-- do (migration 37): Azul has already moved them once - the fee launched in July 2025 at
-- 90 days with a different price per tier - and a published rule that changes is a value,
-- not a branch. NULL means nobody has configured this program's fee, which is different
-- from a fee of zero: nothing is computed at all in that case.
ALTER TABLE loyalty_programs ADD COLUMN redemption_fee_domestic_cents      INTEGER NULL;
ALTER TABLE loyalty_programs ADD COLUMN redemption_fee_international_cents INTEGER NULL;
ALTER TABLE loyalty_programs ADD COLUMN redemption_fee_advance_days        INTEGER NULL;

-- Deliberately "waives the fee" rather than "is Diamante Unique". The tier is Azul's
-- vocabulary for one program; what the calculation actually needs to know is whether this
-- account pays. Note that R$ 69,90 is NOT specific to Diamante - base, Topázio, Safira and
-- Diamante all pay the same domestic third-party fee today. Unique is what buys the waiver.
--
-- FALSE is what every existing row means, and a default keeps this off the table rewrite
-- path on a populated database.
ALTER TABLE accounts ADD COLUMN waives_redemption_fee BOOLEAN NOT NULL DEFAULT FALSE;

-- Only the saving is recorded, not what a paying account owed. The point of the column is
-- to price the decision of issuing on a Unique account rather than an ordinary one; what an
-- ordinary account paid the airline is not settled through this system and has no reader, so
-- storing it would be a number nobody reconciles.
--
-- NULL, not zero, and NULL on every ordinary account: an issuance the rule did not apply to
-- has no saving to report, and zero would claim it was computed and came to nothing.
ALTER TABLE issuances ADD COLUMN redemption_fee_saved_cents INTEGER NULL;
-- Set the moment an operator types the amount by hand, and what stops the automatic
-- recalculation from overwriting the correction on the next Traveos sync. Clearing the
-- cell turns it back off and hands the row back to the rule.
ALTER TABLE issuances ADD COLUMN redemption_fee_manual      BOOLEAN NOT NULL DEFAULT FALSE;

-- The published values as of today, for the Azul program already in this database. New
-- environments get the same numbers from seeds/loyalty_programs.json instead, since the
-- seeder runs after the migrations and creates the programs from scratch there.
--
-- Matched on the aliases too: an operator may have registered it as "TudoAzul" long before
-- the seeder existed - same reasoning as migration 37.
--
-- The international value is Diamante's. We model the waiver, not the tiers, so an Azul
-- account on a lower tier would owe more than this says; it is editable on the program
-- without a deploy precisely so that stays fixable.
UPDATE loyalty_programs
   SET redemption_fee_domestic_cents      = 6990,
       redemption_fee_international_cents = 17490,
       redemption_fee_advance_days        = 120
 WHERE UPPER(code) IN ('AZUL', 'TUDOAZUL') OR UPPER(name) IN ('AZUL', 'TUDOAZUL', 'TUDO AZUL');
