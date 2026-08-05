-- +goose Up
UPDATE pfas.policy_rule_packs
SET review_status = 'RETIRED'
WHERE code = 'MI-PFAS-BIOSOLIDS'
  AND version = '2024.2'
  AND review_status = 'ACTIVE';

-- +goose Down
UPDATE pfas.policy_rule_packs
SET review_status = 'RETIRED'
WHERE code = 'MI-PFAS-BIOSOLIDS'
  AND version = '2024.3'
  AND review_status = 'ACTIVE';

UPDATE pfas.policy_rule_packs
SET review_status = 'ACTIVE'
WHERE code = 'MI-PFAS-BIOSOLIDS'
  AND version = '2024.2'
  AND review_status = 'RETIRED';
