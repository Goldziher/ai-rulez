import sqlite3, sys

body = sys.argv[1]
cur = sqlite3.connect("orders.db").cursor()
cur.execute(f"SELECT id, total FROM orders WHERE note = '{body}'")
print(cur.fetchall())
