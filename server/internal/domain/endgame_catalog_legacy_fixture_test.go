package domain

// Old mutation-focused unit tests retain explicit, test-only aliases. Production
// never sees these globals; DB-built catalogs remain independently owned.
var uniqueDefinitions = testEndgameCatalog().Uniques(UniqueContentVersion)
var uniqueByBase = testEndgameCatalog().uniques[UniqueContentVersion]
var legacyUniqueDefinitions = testEndgameCatalog().Uniques(0)
var legacyUniqueByBase = testEndgameCatalog().uniques[0]
var legacyV5UniqueDefinitions = testEndgameCatalog().Uniques(5)
var legacyV5UniqueByBase = testEndgameCatalog().uniques[5]
