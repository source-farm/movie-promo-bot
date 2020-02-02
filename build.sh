SQLITE_LOCATION=sqlite/lib

echo "building SQLite library..."
pushd $SQLITE_LOCATION 1>/dev/null
gcc -DSQLITE_OMIT_LOAD_EXTENSION -DSQLITE_DEFAULT_FOREIGN_KEYS=1 -c sqlite3.c
ar rcs libsqlite3.a sqlite3.o
rm sqlite3.o
popd 1>/dev/null
echo "SQLite library build finished"

echo "building movie-promo-bot"
go build -o movie-promo-bot .
echo "movie-promo-bot build finished"

rm $SQLITE_LOCATION/libsqlite3.a
