package jsonstream

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
)

// TODO: придумать другой разделитель ключей, т.к. JSON ключ сам по себе может
// содержать запятую.
const keyDelim = ","

type jsonElement byte

const (
	jsonObject jsonElement = iota
	jsonArray
	jsonKey
)

type jsonPath string

func (j *jsonPath) push(key string) {
	if *j == "" {
		*j = jsonPath(key)
	} else {
		*j = *j + jsonPath(keyDelim+key)
	}
}

func (j *jsonPath) pop() {
	delimPos := strings.LastIndex(string(*j), keyDelim)
	if delimPos != -1 {
		*j = jsonPath(string(*j)[:delimPos])
	} else {
		*j = ""
	}
}

type pathValue struct {
	ptr   interface{}
	found bool
}

// Scanner сканирует входной JSON поток в поиске указанных значений.
type Scanner struct {
	paths map[string]pathValue
}

// NewScanner создаёт новый сканер JSON потока.
func NewScanner() *Scanner {
	scanner := Scanner{
		paths: map[string]pathValue{},
	}
	return &scanner
}

// SearchFor задаёт путь path из JSON ключей, значение в конце которой
// декодируется в v. v должен быть указателем. Если path равен nil, то в v
// декодируется весь JSON поток. Если path не равен nil, то он не должен содержать
// пустых строк, т.е. ключей. Путь в path определяется последовательным
// перечислением ключей, где каждый следующий переходит на один уровень вглубь.
//
// Пример:
//
//	city := `
//	{
//		"name": "Venice",
//		"location": {
//			"lat": 45.4333,
//			"long": 12.3167
//		}
//	}`
//
//	scanner := NewScanner()
//	var latitude float64
//	scanner.SearchFor(&latitude, "location", "lat")
//	scanner.Find(strings.NewReader(city))
//	fmt.Println(latitude)
//
// Нельзя добавлять путь, который является продолжением уже ранее добавленного
// пути.
//
// При добавлении более общего пути ранее добавленные продолжения этого пути
// удаляются из сканера.
//
func (s *Scanner) SearchFor(v interface{}, path ...string) error {
	vType := reflect.TypeOf(v)
	if vType == nil || vType.Kind() != reflect.Ptr {
		return errors.New("jsonstream: v isn't a pointer")
	}

	if path == nil {
		s.paths[""] = pathValue{ptr: v, found: false}
	} else {
		for i := range path {
			if path[i] == "" {
				return errors.New("jsonstream: path contains empty key")
			}
		}

		vPath := strings.Join(path, keyDelim)

		for p := range s.paths {
			// Исключаем добавление пути, если этот путь является продолжением
			// другого ранее добавленного пути.
			if strings.HasPrefix(vPath, p) && vPath != p {
				pathExpanded := strings.Join(path, " -> ")
				pExpanded := strings.ReplaceAll(p, keyDelim, " -> ")
				if pExpanded == "" {
					pExpanded = "<empty path>"
				}
				return errors.New("jsonstream: path (" + pathExpanded + ") is a continuation of the path (" + pExpanded + ")")
			}

			// Удаляем из ранее добавленных путей все пути, которые являются
			// продолжением path.
			if strings.HasPrefix(p, vPath) {
				delete(s.paths, p)
			}
		}

		s.paths[vPath] = pathValue{ptr: v, found: false}
	}

	return nil
}

// Find запускает сканирование JSON потока. Если все указанные через SearchFor
// значения найдены, то возвращается nil. Иначе возвращается ошибка.
func (s *Scanner) Find(stream io.Reader) error {
	dec := json.NewDecoder(stream)
	// Пустая строка в paths означает, что нужно декодировать весь JSON.
	// В этом случае сканер превращается в json.Unmarshaler.
	if value, ok := s.paths[""]; ok {
		return dec.Decode(value.ptr)
	}

	stack := []jsonElement{}
	var path jsonPath
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("jsonstream: error while parsing stream")
		}

		switch token := token.(type) {
		case json.Delim:
			switch token {
			case '{':
				stack = append(stack, jsonObject)
			case '[':
				stack = append(stack, jsonArray)
			case '}', ']':
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
				if len(stack) > 0 && stack[len(stack)-1] == jsonKey {
					// Завершили работу с объектом или массивом. Если они были
					// связаны с ключом, то удаляем ключ из стека тоже.
					stack = stack[:len(stack)-1]
					path.pop()
				}
			}

		default:
			if len(stack) > 0 {
				switch stack[len(stack)-1] {
				case jsonObject:
					// Нашли ключ.
					stack = append(stack, jsonKey)
					key, ok := token.(string)
					if ok {
						path.push(key)
						if value, ok := s.paths[string(path)]; ok && !value.found {
							err := dec.Decode(value.ptr)
							if err != nil {
								return err
							}
							value.found = true
							s.paths[string(path)] = value
							// Завершили работу с значением ключа.
							stack = stack[:len(stack)-1]
							path.pop()
						}
					}
				case jsonArray:
					// Нашли элемент массива.
				case jsonKey:
					// Завершили работу с значением ключа.
					stack = stack[:len(stack)-1]
					path.pop()
				}
			}
		}
	}

	for _, value := range s.paths {
		if !value.found {
			return errors.New("jsonstream: not all values found")
		}
	}

	return nil
}

// Reset сбрасывает сканер к начальному состоянию.
func (s *Scanner) Reset() {
	s.paths = map[string]pathValue{}
}
