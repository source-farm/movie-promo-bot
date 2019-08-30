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

// Scanner сканирует входной JSON поток в поиске указанных значений.
type Scanner struct {
	paths map[string]interface{}
}

// NewScanner создаёт новый сканер JSON потока.
func NewScanner() *Scanner {
	scanner := Scanner{
		paths: map[string]interface{}{},
	}
	return &scanner
}

// SearchFor задаёт путь path из JSON ключей, значение в конце которой
// записывается в v. v должен быть указателем. Если path равен nil, то в v
// записывается значение прямо в корне JSON'а. Если path не равен nil, то он не
// должен содержать пустых строк, т.е. ключей. Путь в path определяется
// последовательным перечислением ключей, где каждый следующий переходит на один
// уровень вглубь.
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
func (s *Scanner) SearchFor(v interface{}, path ...string) error {
	// TODO: исключить добавление вложенных путей.

	vType := reflect.TypeOf(v)
	if vType == nil || vType.Kind() != reflect.Ptr {
		return errors.New("jsonstream: v isn't a pointer")
	}

	for i := range path {
		if path[i] == "" {
			return errors.New("jsonstream: path contains empty key")
		}
	}

	vPath := strings.Join(path, keyDelim)
	s.paths[vPath] = v

	return nil
}

// Find запускает сканирование JSON потока. Если все указанные через SearchFor
// значения найдены, то возвращается nil. Иначе возвращается ошибка.
func (s *Scanner) Find(stream io.Reader) error {
	dec := json.NewDecoder(stream)
	// Пустая строка в paths означает, что нужно декодировать весь JSON.
	// В этом случае сканер превращается в json.Unmarshaler.
	if v, ok := s.paths[""]; ok {
		return dec.Decode(v)
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
						if v, ok := s.paths[string(path)]; ok {
							err := dec.Decode(v)
							if err != nil {
								return err
							}
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

	return nil
}

// Reset сбрасывает сканер к начальному состоянию.
func (s *Scanner) Reset() {
	s.paths = map[string]interface{}{}
}
