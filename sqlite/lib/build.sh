gcc -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_DEFAULT_FOREIGN_KEYS=1 -c sqlite3.c
ar rcs libsqlite3.a sqlite3.o
rm sqlite3.o
