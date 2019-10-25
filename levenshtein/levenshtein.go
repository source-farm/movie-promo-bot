package levenshtein

// Distance находит расстояние Левенштейна: стоимость приведения строки source
// к строке target, если вставка, удаление и замена одного символа стоит
// insCost, delCost и subCost соответственно.
func Distance(source, target string, insCost, delCost, subCost int) int {
	sr := []rune(source)
	tr := []rune(target)

	prevRow := make([]int, len(tr)+1)
	curRow := make([]int, len(tr)+1)
	for j := 0; j <= len(tr); j++ {
		prevRow[j] = j * insCost
	}

	for i := 1; i <= len(sr); i++ {
		curRow[0] = i * delCost
		for j := 1; j <= len(tr); j++ {
			deletionCost := prevRow[j] + delCost
			insertionCost := curRow[j-1] + insCost
			substitutionCost := prevRow[j-1]
			if sr[i-1] != tr[j-1] {
				substitutionCost += subCost
			}
			curRow[j] = min(deletionCost, insertionCost, substitutionCost)
		}
		prevRow, curRow = curRow, prevRow
	}

	return prevRow[len(prevRow)-1]
}

func min(a, b, c int) int {
	if a > b {
		if b > c {
			return c
		}
		return b
	}

	if a < c {
		return a
	}
	return c
}
