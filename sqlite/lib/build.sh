gcc -DSQLITE_OMIT_LOAD_EXTENSION -c sqlite3.c -o sqlite3.o
ar rcs libsqlite3.a sqlite3.o
rm sqlite3.o
