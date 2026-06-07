# Database

Production: Cloudflare D1, `krcrackers-products` (`735027ae-2327-4561-8e62-538973817b06`), region APAC.

Schema (matches prod exactly: all fields except `id` are nullable):

```sql
CREATE TABLE products (
    id            INTEGER PRIMARY KEY,
    name          TEXT,
    price         REAL,
    brand         TEXT,
    description   TEXT,
    category      TEXT,
    image         TEXT,
    compare_price REAL
);
```

Applied versions are tracked in a `schema_migrations` table that the runner creates automatically.

12 categories in the current data: Bombs, Chakras, Crackers, Fancy, Flower Pots, Gift Boxes, Lar, Laxmi & Kuruvi, Night Crackers, Rocket, Shots, Sparkles.
