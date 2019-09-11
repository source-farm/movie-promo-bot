gcc -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_DEFAULT_FOREIGN_KEYS=1 -c sqlite3.c -o sqlite3.o
ar rcs libsqlite3.a sqlite3.o
rm sqlite3.o
