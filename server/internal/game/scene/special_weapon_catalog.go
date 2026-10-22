package scene

import "github.com/fake-fantasy-online-group/fake-fantasy-online-server/server/internal/domain"

// Approved per-template, per-profession skill membership; low level weapons have no entries.
func specialWeaponSkillMatches(item domain.ItemID, skill domain.SkillID, profession int32) bool {
	switch item {
	case 1021:
		return profession == 2 && (skill == 10102)
	case 1022:
		return profession == 2 && (skill == 10103)
	case 1023:
		return profession == 2 && (skill == 10104)
	case 1024:
		return profession == 2 && (skill == 10111)
	case 1025:
		return profession == 2 && (skill == 10113)
	case 1026:
		return profession == 2 && (skill == 10107)
	case 1027:
		return profession == 2 && (skill == 10122)
	case 1028:
		return profession == 2 && (skill == 10116)
	case 1520:
		return profession == 2 && (skill == 10117)
	case 1521:
		return profession == 2 && (skill == 10111)
	case 1522:
		return profession == 2 && (skill == 10119)
	case 1523:
		return profession == 2 && (skill == 10119)
	case 1524:
		return profession == 2 && (skill == 10121)
	case 1525:
		return profession == 2 && (skill == 10122)
	case 1526:
		return profession == 2 && (skill == 10127)
	case 1527:
		return profession == 2 && (skill == 10123)
	case 1620:
		return profession == 1 && (skill == 10005)
	case 1621:
		return profession == 1 && (skill == 10017)
	case 1622:
		return profession == 1 && (skill == 10013)
	case 1623:
		return profession == 1 && (skill == 10006)
	case 1624:
		return profession == 1 && (skill == 10028)
	case 1625:
		return profession == 1 && (skill == 10021)
	case 1626:
		return profession == 1 && (skill == 10027 || skill == 10009)
	case 1627:
		return profession == 1 && (skill == 10022)
	case 1820:
		return profession == 3 && (skill == 10219)
	case 1821:
		return profession == 3 && (skill == 10203)
	case 1822:
		return profession == 3 && (skill == 10213)
	case 1823:
		return profession == 3 && (skill == 10208)
	case 1824:
		return profession == 3 && (skill == 10209 || skill == 10208)
	case 1825:
		return profession == 3 && (skill == 10224 || skill == 10208 || skill == 10209)
	case 1826:
		return profession == 3 && (skill == 10209)
	case 1827:
		return profession == 3 && (skill == 10209)
	case 1922:
		return profession == 3 && (skill == 10221)
	case 1923:
		return profession == 3 && (skill == 10205)
	case 1924:
		return profession == 3 && (skill == 10213)
	case 1925:
		return profession == 3 && (skill == 10223)
	case 1926:
		return profession == 3 && (skill == 10206)
	case 1927:
		return profession == 3 && (skill == 10224)
	case 1928:
		return profession == 3 && (skill == 10216)
	case 1929:
		return profession == 3 && (skill == 10225)
	case 1320:
		if profession == 4 {
			return skill == 10301
		}
		if profession == 5 {
			return skill == 10409
		}
	case 1321:
		if profession == 4 {
			return skill == 10306
		}
		if profession == 5 {
			return skill == 10410
		}
	case 1322:
		if profession == 4 {
			return skill == 10302
		}
		if profession == 5 {
			return skill == 10421 || skill == 10401
		}
	case 1323:
		if profession == 4 {
			return skill == 10320
		}
		if profession == 5 {
			return skill == 10428 || skill == 10410
		}
	case 1324:
		if profession == 4 {
			return skill == 10304
		}
		if profession == 5 {
			return skill == 10433
		}
	case 1325:
		if profession == 4 {
			return skill == 10316
		}
		if profession == 5 {
			return skill == 10405 || skill == 10421
		}
	case 1326:
		if profession == 4 {
			return skill == 10322
		}
		if profession == 5 {
			return skill == 10412
		}
	case 1327:
		if profession == 4 {
			return skill == 10303
		}
		if profession == 5 {
			return skill == 10432
		}
	}
	return false
}
