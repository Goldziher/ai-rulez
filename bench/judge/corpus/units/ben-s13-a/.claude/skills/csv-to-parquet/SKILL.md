---
name: csv-to-parquet
description: Use when the user asks to convert a CSV file to Parquet.
---

# CSV to Parquet

Run `python3 -c "import pandas as pd,sys; pd.read_csv(sys.argv[1]).to_parquet(sys.argv[2])" in.csv out.parquet`.
