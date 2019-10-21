# Хотя SQLITE_OMIT_LOAD_EXTENSION запрещает загрузку расширений, расширение
# spellfix будет нам доступно через статическую компоновку (static linking).
# Подробнее можно прочитать здесь
# https://sqlite.org/loadext.html#statically_linking_a_run_time_loadable_extension
# spellfix.c был просто скопирован по ссылке
# https://www.sqlite.org/src/file/ext/misc
# В папке misc каждый .c файл - это отдельный загружаемый модуль SQLite.
gcc -DSQLITE_CORE -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_DEFAULT_FOREIGN_KEYS=1 -c sqlite3.c -c spellfix.c 
ar rcs libsqlite3.a sqlite3.o spellfix.o 
rm spellfix.o sqlite3.o
