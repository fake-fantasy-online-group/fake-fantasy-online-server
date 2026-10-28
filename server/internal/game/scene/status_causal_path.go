package scene

// Copied statuses retain their ancestry, while a new producer contributes its
// current path. Always allocate: stored status snapshots must be immutable.
func mergeStatusCausalPath(stored, current []string) []string {
	path := append([]string(nil), stored...)
	for _, key := range current {
		found := false
		for _, previous := range path {
			if previous == key {
				found = true
				break
			}
		}
		if !found {
			path = append(path, key)
		}
	}
	return path
}
