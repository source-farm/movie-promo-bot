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

// Filter используется для фильтрации JSON массива.
type Filter = func(v interface{}) bool

type pathValue struct {
	ptr    interface{}
	found  bool
	filter Filter
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

// SetFilter позволяет фильтровать значения JSON массивов. Путь path уже должен
// существовать в сканере и при этом установленное для него через метод
// SearchFor значение должно быть указателем на слайс.
func (s *Scanner) SetFilter(filter Filter, path ...string) error {
	valuePath := strings.Join(path, keyDelim)
	value, ok := s.paths[valuePath]
	if !ok {
		return errors.New("jsonstream: path not found")
	}
	if reflect.TypeOf(value.ptr).Elem().Kind() != reflect.Slice {
		return errors.New("jsonstream: path isn't slice")
	}
	value.filter = filter
	s.paths[valuePath] = value
	return nil
}

// Find запускает сканирование JSON потока. Если все указанные через SearchFor
// пути найдены, то возвращается nil. Нахождение пути необъязательно означает,
// что значение по этому пути было правильно декодировано.
func (s *Scanner) Find(stream io.Reader) error {
	dec := json.NewDecoder(stream)
	// Пустая строка в paths означает, что нужно декодировать весь JSON. Так и
	// поступаем, только если JSON не декодируется в слайс. Этот случай
	// рассматривается отдельно в основном цикле ниже, т.к. для массивов может
	// быть установлена фильтрация.
	value, ok := s.paths[""]
	if ok && reflect.TypeOf(value.ptr).Elem().Kind() != reflect.Slice {
		return dec.Decode(value.ptr)
	}

	var stack []jsonElement
	var path jsonPath
	for {
		// TODO: остановить декодирование, если все пути найдены.

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
				value, ok := s.paths[string(path)]
				if ok && !value.found {
					// reflect.TypeOf(value.ptr) - это указатель (*T), т.к. SearchFor позволяет добавлять
					// только указатели для путей.
					// reflect.TypeOf(value.ptr).Elem() - это T.
					valueType := reflect.TypeOf(value.ptr).Elem()
					if valueType.Kind() == reflect.Slice {
						// В следующих двух строках создаётся значение такого же типа, как и элементы
						// слайса, в который нужно добавлять декодированные значения и его адрес
						// записывается в decodedValuePtr.
						sliceElementType := valueType.Elem()
						decodedValuePtr := reflect.New(sliceElementType)
						for dec.More() { // Пока в массиве есть элементы.
							// decodedValuePtr.Interface() позволяет перейти из области reflect'а обратно в Go.
							err := dec.Decode(decodedValuePtr.Interface())
							if err != nil {
								return err
							}
							if value.filter == nil || value.filter(decodedValuePtr.Elem().Interface()) {
								sliceValuePtr := reflect.ValueOf(value.ptr)
								// sliceValuePtr.CanSet() возвращает false.
								// sliceValuePtr.Elem() возвращает reflect.Value, который позволяет менять значение,
								// на которое указывает value.ptr. Более подробно можно прочитать по ссылке
								//
								// https://blog.golang.org/laws-of-reflection
								//
								// в третьем законе - "To modify a reflection object, the value must be settable".
								//
								sliceValue := sliceValuePtr.Elem()
								// decodedValuePtr - это *T, decodedValuePtr.Elem() - это T.
								sliceValue.Set(reflect.Append(sliceValue, decodedValuePtr.Elem()))
							}
						}
					} else {
						return errors.New("jsonstream: cannot decode array to non slice value")
					}
					value.found = true
					s.paths[string(path)] = value
				}
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
						value, ok := s.paths[string(path)]
						// Здесь декодируются все JSON значения, кроме массивов. Массивы обрабатываются выше.
						if ok && !value.found && reflect.TypeOf(value.ptr).Elem().Kind() != reflect.Slice {
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
			return errors.New("jsonstream: not all paths found")
		}
	}

	return nil
}

// Reset сбрасывает сканер к начальному состоянию.
func (s *Scanner) Reset() {
	s.paths = map[string]pathValue{}
}
