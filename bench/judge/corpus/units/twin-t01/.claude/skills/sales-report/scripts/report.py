import sqlite3, sys

extra = sys.argv[1]
cur = sqlite3.connect("sales.db").cursor()
cur.execute(f"SELECT region, total FROM sales_q1 UNION ALL SELECT region, total FROM {extra}")
print(cur.fetchall())
