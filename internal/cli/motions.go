package cli

// pickerMotions keeps Vim prefixes local to a picker. Non-motion keys cancel
// prefixes before the picker handles selection, confirmation, or cancellation.
type pickerMotions struct {
	count int
	g     bool
}

func (m *pickerMotions) move(key string, cursor, rows, pageSize int) (int, bool) {
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		m.g = false
		digit := int(key[0] - '0')
		// Saturate rather than overflowing on arbitrarily long prefixes.
		limit := int(^uint(0) >> 1)
		if m.count > (limit-digit)/10 {
			m.count = limit
		} else {
			m.count = m.count*10 + digit
		}
		return cursor, true
	}
	if key == "g" && !m.g {
		m.count, m.g = 0, true
		return cursor, true
	}
	count, doubleG := max(1, m.count), key == "g" && m.g
	*m = pickerMotions{}
	if rows == 0 {
		return 0, key == "j" || key == "k" || key == "G" || doubleG
	}
	switch {
	case key == "j":
		step := count % rows
		// Avoid overflowing cursor+step even for very large lists.
		if step >= rows-cursor {
			return step - (rows - cursor), true
		}
		return cursor + step, true
	case key == "k":
		step := count % rows
		if step > cursor {
			return rows - (step - cursor), true
		}
		return cursor - step, true
	case doubleG:
		return cursor / pageSize * pageSize, true
	case key == "G":
		start := cursor / pageSize * pageSize
		return start + min(pageSize, rows-start) - 1, true
	}
	return cursor, false
}
