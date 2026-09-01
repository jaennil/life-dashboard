-- How habitual a food is, taken from the order foods.get_most_eaten returns.
--
-- Frequency is the tie-break that matters. The account has three products just
-- called "Банан" - from ВкусВилл, Пятерочка and Дикси - and a phrase that says
-- only "банан" has to land on one of them. The same signal is what let the
-- exercise matcher pick correctly on the first try.
--
-- NULL means the food never appeared in the most-eaten lists, only in the recent
-- ones: eaten lately, but not habitually.
ALTER TABLE fatsecret_foods ADD COLUMN IF NOT EXISTS most_eaten_rank integer;
