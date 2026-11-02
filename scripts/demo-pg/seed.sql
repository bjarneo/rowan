-- The Rowan demo database: about 46 million rows in 4 schemas.
-- scripts/demo-pg/start.sh loads it into a PostgreSQL container.
--
-- The data tests the parts of Rowan that a small database does not reach:
-- row estimates on large tables, a partitioned table, a materialized view,
-- types such as jsonb, uuid and arrays, names that need quotes, a table with
-- 120 columns, and a schema with more than 20,000 columns.

\set ON_ERROR_STOP on
\timing on
SET synchronous_commit = off;
SET maintenance_work_mem = '1GB';
SET max_parallel_maintenance_workers = 4;
SELECT setseed(0.42);

CREATE SCHEMA billing;
CREATE SCHEMA analytics;
CREATE SCHEMA legacy;

CREATE TYPE order_status AS ENUM ('paid', 'shipped', 'refunded', 'cancelled');

CREATE FUNCTION pick(a text[], r double precision) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$ SELECT a[1 + floor(r * array_length(a, 1))::int] $$;

-- ---- public ----

CREATE TABLE customers (
  id         bigint PRIMARY KEY,
  name       text NOT NULL,
  email      text NOT NULL,
  country    char(2) NOT NULL,
  plan       text NOT NULL,
  mrr_cents  integer NOT NULL,
  created_at timestamptz NOT NULL
);
INSERT INTO customers
SELECT g,
  pick('{Liam,Ines,Zoe,Omar,Yuki,Lena,Kai,Noah,Maja,Aiko,Emil,Sara,Jonas,Nora,Ali,Mei,Lucas,Ida,Ravi,Elin,Hugo,Amara,Felix,Sofia}', r1) || ' ' ||
  pick('{Silva,Chen,Lindqvist,Ivanova,Dubois,Muller,Okafor,Andersen,Haugen,Tanaka,Novak,Rossi,Berg,Kowalski,Nakamura,Larsen,Moreau,Costa,Patel,Jensen,Fischer,Garcia,Kim,Olsen}', r2),
  'user' || g || '@' || pick('{acme.io,fastmail.com,gmail.com,proton.me,example.org}', r3),
  pick('{NO,SE,DK,DE,NL,GB,US,CA,FR,ES,PL,JP}', r4),
  pick('{free,free,free,starter,starter,team,business,enterprise}', r5),
  (r5 * 90000)::int,
  now() - interval '1500 days' * r6
FROM (SELECT g, random() r1, random() r2, random() r3, random() r4, random() r5, random() r6 FROM generate_series(1, 1000000) g) s;

CREATE TABLE products (
  id          integer PRIMARY KEY,
  sku         text NOT NULL UNIQUE,
  name        text NOT NULL,
  price_cents integer NOT NULL,
  active      boolean NOT NULL DEFAULT true,
  tags        text[] NOT NULL DEFAULT '{}'
);
INSERT INTO products
SELECT g, 'SKU-' || lpad(g::text, 6, '0'),
  pick('{Desk,Lamp,Chair,Mug,Notebook,Cable,Dock,Stand,Mat,Shelf,Clock,Bag}', r1) || ' ' || pick('{mini,pro,max,lite,plus,one,go,air}', r2) || ' ' || g,
  (500 + r3 * 49500)::int, r4 > 0.08,
  ARRAY[pick('{office,home,travel,kitchen,audio,cables}', r1), pick('{new,sale,eco,classic}', r2)]
FROM (SELECT g, random() r1, random() r2, random() r3, random() r4 FROM generate_series(1, 50000) g) s;

CREATE TABLE orders (
  id          bigint PRIMARY KEY,
  customer_id bigint NOT NULL,
  status      order_status NOT NULL,
  total_cents integer NOT NULL,
  currency    char(3) NOT NULL,
  placed_at   timestamptz NOT NULL,
  shipped_at  timestamptz
);
INSERT INTO orders
SELECT g, 1 + floor(r1 * 1000000)::bigint,
  (CASE WHEN r2 < 0.55 THEN 'paid' WHEN r2 < 0.85 THEN 'shipped' WHEN r2 < 0.9 THEN 'refunded' ELSE 'cancelled' END)::order_status,
  (900 + r3 * 60000)::int, pick('{USD,USD,EUR,EUR,GBP,NOK}', r4),
  now() - interval '730 days' * r5,
  CASE WHEN r2 >= 0.55 AND r2 < 0.85 THEN now() - interval '730 days' * r5 + interval '5 days' * r6 END
FROM (SELECT g, random() r1, random() r2, random() r3, random() r4, random() r5, random() r6 FROM generate_series(1, 6000000) g) s;

CREATE TABLE order_items (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  order_id   bigint NOT NULL,
  product_id integer NOT NULL,
  quantity   smallint NOT NULL,
  unit_cents integer NOT NULL
);
INSERT INTO order_items (order_id, product_id, quantity, unit_cents)
SELECT o, 1 + floor(random() * 50000)::int, 1 + floor(random() * 4)::smallint, (500 + random() * 49500)::int
FROM generate_series(1, 6000000) o, generate_series(1, 1 + (o % 5)::int) n;

CREATE TABLE "Audit Log" (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  "Event Type" text NOT NULL,
  "user"       text NOT NULL,
  at           timestamptz NOT NULL,
  details      jsonb
);
INSERT INTO "Audit Log" ("Event Type", "user", at, details)
SELECT pick('{login,logout,export,unlock,approve,deny}', r1), 'user' || (1 + floor(r2 * 5000)::int) || '@acme.io',
  now() - interval '365 days' * r3, jsonb_build_object('ip', '10.0.' || (g % 255) || '.' || (g % 7), 'ok', r1 > 0.1)
FROM (SELECT g, random() r1, random() r2, random() r3 FROM generate_series(1, 100000) g) s;

CREATE TABLE type_zoo (
  id        uuid PRIMARY KEY,
  amount    numeric(12, 2),
  ratio     double precision,
  flags     integer[],
  labels    text[],
  doc       jsonb,
  raw       bytea,
  addr      inet,
  span      interval,
  day       date,
  at_time   time,
  enabled   boolean,
  search    tsvector,
  location  point,
  big       bigint,
  note      text
);
INSERT INTO type_zoo
SELECT md5(g::text)::uuid, round((r1 * 100000)::numeric, 2), r2, ARRAY[g % 7, g % 11, g % 13], ARRAY['a' || g % 5, 'b' || g % 3],
  jsonb_build_object('id', g, 'tags', jsonb_build_array('x', 'y'), 'nested', jsonb_build_object('depth', g % 4, 'ok', true)),
  decode(md5(g::text), 'hex'), ('192.168.' || g % 255 || '.' || g % 250)::inet, make_interval(hours => g % 48), current_date - g % 900,
  make_time(g % 24, g % 60, 0), g % 2 = 0, to_tsvector('english', 'order ' || g || ' shipped late'), point(r1 * 180, r2 * 90),
  (r3 * 9e15)::bigint, CASE WHEN g % 10 = 0 THEN NULL WHEN g % 10 = 1 THEN repeat('long text ', 80) ELSE 'note ' || g END
FROM (SELECT g, random() r1, random() r2, random() r3 FROM generate_series(1, 10000) g) s;

CREATE TABLE support_tickets (
  id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  customer_id bigint NOT NULL,
  subject     text NOT NULL,
  body        text NOT NULL,
  opened_at   timestamptz NOT NULL,
  closed_at   timestamptz
);
INSERT INTO support_tickets (customer_id, subject, body, opened_at, closed_at)
SELECT 1 + floor(r1 * 1000000)::bigint, pick('{Refund request,Late delivery,Invoice copy,Login issue,Wrong item,Change plan}', r2),
  repeat(pick('{"The parcel did not arrive. ","I need a copy of the invoice. ","Please change the billing address. ","The order shows the wrong total. "}', r3), 12 + (g % 20)),
  now() - interval '400 days' * r4, CASE WHEN r5 < 0.8 THEN now() - interval '400 days' * r4 + interval '3 days' * r5 END
FROM (SELECT g, random() r1, random() r2, random() r3, random() r4, random() r5 FROM generate_series(1, 200000) g) s;

-- A table with 120 columns tests wide grids and the row inspector.
DO $$
DECLARE cols text := '';
BEGIN
  FOR i IN 1..119 LOOP
    cols := cols || format(', metric_%s %s', lpad(i::text, 3, '0'), CASE i % 3 WHEN 0 THEN 'integer' WHEN 1 THEN 'double precision' ELSE 'text' END);
  END LOOP;
  EXECUTE 'CREATE TABLE wide_metrics (id integer PRIMARY KEY' || cols || ')';
  EXECUTE 'INSERT INTO wide_metrics SELECT g' || (SELECT string_agg(CASE i % 3 WHEN 0 THEN ', (g * ' || i || ') % 1000' WHEN 1 THEN ', random() * ' || i ELSE ', ''v'' || g % ' || (i + 1) END, '' ORDER BY i) FROM generate_series(1, 119) i) || ' FROM generate_series(1, 5000) g';
END $$;

-- ---- billing ----

CREATE TABLE billing.invoices (
  id        bigint PRIMARY KEY,
  order_id  bigint NOT NULL,
  number    text NOT NULL,
  issued_at timestamptz NOT NULL,
  paid_at   timestamptz
);
INSERT INTO billing.invoices
SELECT g, g, 'INV-' || to_char(now() - interval '730 days' * r1, 'YYYY') || '-' || lpad(g::text, 7, '0'),
  now() - interval '730 days' * r1, CASE WHEN r2 < 0.93 THEN now() - interval '730 days' * r1 + interval '20 days' * r2 END
FROM (SELECT g, random() r1, random() r2 FROM generate_series(1, 5000000) g) s;

CREATE TABLE billing.payments (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  invoice_id   bigint NOT NULL,
  amount_cents integer NOT NULL,
  method       text NOT NULL,
  created_at   timestamptz NOT NULL
);
INSERT INTO billing.payments (invoice_id, amount_cents, method, created_at)
SELECT 1 + floor(r1 * 5000000)::bigint, (900 + r2 * 60000)::int, pick('{card,card,card,invoice,paypal,klarna}', r3), now() - interval '730 days' * r4
FROM (SELECT g, random() r1, random() r2, random() r3, random() r4 FROM generate_series(1, 4000000) g) s;

-- ---- analytics: a table with one partition per month ----

CREATE TABLE analytics.events (
  id          bigint NOT NULL,
  occurred_at timestamptz NOT NULL,
  customer_id bigint,
  kind        text NOT NULL,
  payload     jsonb
) PARTITION BY RANGE (occurred_at);
DO $$
DECLARE m date := date_trunc('month', now() - interval '730 days')::date;
BEGIN
  WHILE m <= date_trunc('month', now() + interval '1 month')::date LOOP
    EXECUTE format('CREATE TABLE analytics.%I PARTITION OF analytics.events FOR VALUES FROM (%L) TO (%L)',
      'events_' || to_char(m, 'YYYY_MM'), m, (m + interval '1 month')::date);
    m := (m + interval '1 month')::date;
  END LOOP;
END $$;
INSERT INTO analytics.events
SELECT g, now() - interval '729 days' * r1, 1 + floor(r2 * 1000000)::bigint, pick('{page_view,page_view,page_view,search,add_to_cart,checkout,login}', r3),
  jsonb_build_object('path', pick('{/,/pricing,/docs,/cart,/account,/orders}', r4), 'ms', (r5 * 900)::int, 'mobile', r2 < 0.4)
FROM (SELECT g, random() r1, random() r2, random() r3, random() r4, random() r5 FROM generate_series(1, 12000000) g) s;

-- ---- legacy: 1,100 tables with 20 columns each, 22,000 columns ----

DO $$
BEGIN
  FOR i IN 1..1100 LOOP
    EXECUTE format('CREATE TABLE legacy.%I (id serial PRIMARY KEY, %s)', 'tbl_' || lpad(i::text, 4, '0'),
      (SELECT string_agg(format('%I %s', 'field_' || lpad(j::text, 2, '0') || '_' || i, CASE j % 4 WHEN 0 THEN 'integer' WHEN 1 THEN 'text' WHEN 2 THEN 'timestamptz' ELSE 'boolean' END), ', ')
       FROM generate_series(1, 19) j));
  END LOOP;
END $$;

-- ---- keys, indexes and views ----

CREATE INDEX ON orders (customer_id);
CREATE INDEX ON orders (placed_at);
CREATE INDEX ON orders (status);
CREATE INDEX ON order_items (order_id);
CREATE INDEX ON billing.invoices (order_id);
CREATE INDEX ON billing.payments (invoice_id);
CREATE INDEX ON support_tickets (customer_id);
ALTER TABLE orders ADD FOREIGN KEY (customer_id) REFERENCES customers (id);
ALTER TABLE order_items ADD FOREIGN KEY (order_id) REFERENCES orders (id);
ALTER TABLE order_items ADD FOREIGN KEY (product_id) REFERENCES products (id);
ALTER TABLE billing.invoices ADD FOREIGN KEY (order_id) REFERENCES orders (id);
ALTER TABLE billing.payments ADD FOREIGN KEY (invoice_id) REFERENCES billing.invoices (id);
ALTER TABLE support_tickets ADD FOREIGN KEY (customer_id) REFERENCES customers (id);

CREATE VIEW v_monthly_revenue AS
SELECT date_trunc('month', placed_at) AS month, currency, sum(total_cents) AS revenue_cents, count(*) AS orders
FROM orders WHERE status IN ('paid', 'shipped') GROUP BY 1, 2;

CREATE MATERIALIZED VIEW analytics.mv_country_revenue AS
SELECT c.country, count(*) AS orders, sum(o.total_cents) AS revenue_cents
FROM orders o JOIN customers c ON c.id = o.customer_id GROUP BY c.country;

VACUUM ANALYZE;
