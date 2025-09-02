package textutil

// SplitN разделяет строку на N частей, по символу разделителя sep.
// Возвращает части строки и флаг ok, который указывает на успешность операции.
// Использовать такую функцию дешевое чем strings.Split - он аллоцирует больше памяти и т.д.
func SplitN(line string, sep byte, numCols int) ([]string, bool) {
	if line == "" {
		return nil, false
	}
	if line[len(line)-1] == '\r' {
		line = line[:len(line)-1] // Убираем лишний символ '\r' для Windows-строк
	}

	// Создаем слайс для колонок
	columns := make([]string, 0, numCols)
	last := 0
	cuts := 0

	// Проходим по строке
	for i := 0; i < len(line); i++ {
		if line[i] == sep {
			columns = append(columns, line[last:i])
			cuts++
			last = i + 1
		}

		// Если нашли больше разделителей, чем нужно, выходим с ошибкой
		if cuts >= numCols {
			return nil, false
		}
	}

	// Добавляем последнюю колонку (после последнего разделителя)
	columns = append(columns, line[last:])

	// Если количество колонок не совпало с ожидаемым, возвращаем false
	if len(columns) != numCols {
		return nil, false
	}

	return columns, true
}
