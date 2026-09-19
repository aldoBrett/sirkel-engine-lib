-- sort_key holds a fractional index (see roci.dev/fracdex). Rows are ordered by (sort_key, id) within their goal
-- (tasks) or task (task_items). COLLATE "C" is required: fractional keys only sort correctly byte by byte.
ALTER TABLE sirkel_engine.tasks
ADD COLUMN IF NOT EXISTS sort_key TEXT COLLATE "C";

ALTER TABLE sirkel_engine.task_items
ADD COLUMN IF NOT EXISTS sort_key TEXT COLLATE "C";

-- Backfill rows that have no key yet, keeping the previous created_at DESC order.
-- Keys are "c" plus three base62 digits, a valid fixed-width fractional index that leaves room for about 238k rows
-- per goal or task.
UPDATE sirkel_engine.tasks t
SET
  sort_key = 'c' || substr(d.digits, (r.n / 3844) % 62 + 1, 1) || substr(d.digits, (r.n / 62) % 62 + 1, 1) || substr(d.digits, r.n % 62 + 1, 1)
FROM
  (
    SELECT
      id,
      (
        row_number() OVER (
          PARTITION BY
            goal_id
          ORDER BY
            created_at DESC,
            id
        ) - 1
      )::int AS n
    FROM
      sirkel_engine.tasks
    WHERE
      sort_key IS NULL
  ) r,
  (
    SELECT
      '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz' AS digits
  ) d
WHERE
  t.id = r.id;

UPDATE sirkel_engine.task_items ti
SET
  sort_key = 'c' || substr(d.digits, (r.n / 3844) % 62 + 1, 1) || substr(d.digits, (r.n / 62) % 62 + 1, 1) || substr(d.digits, r.n % 62 + 1, 1)
FROM
  (
    SELECT
      id,
      (
        row_number() OVER (
          PARTITION BY
            task_id
          ORDER BY
            created_at DESC,
            id
        ) - 1
      )::int AS n
    FROM
      sirkel_engine.task_items
    WHERE
      sort_key IS NULL
  ) r,
  (
    SELECT
      '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz' AS digits
  ) d
WHERE
  ti.id = r.id;

ALTER TABLE sirkel_engine.tasks
ALTER COLUMN sort_key
SET NOT NULL;

ALTER TABLE sirkel_engine.task_items
ALTER COLUMN sort_key
SET NOT NULL;

CREATE INDEX IF NOT EXISTS tasks_goal_id_sort_key_idx ON sirkel_engine.tasks (goal_id, sort_key, id);

CREATE INDEX IF NOT EXISTS task_items_task_id_sort_key_idx ON sirkel_engine.task_items (task_id, sort_key, id);
