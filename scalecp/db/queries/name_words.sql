-- name: RandomAdjective :one
SELECT word
FROM name_words
WHERE kind = 'adjective'
  AND enabled = TRUE
ORDER BY random() LIMIT 1;

-- name: RandomNoun :one
SELECT word
FROM name_words
WHERE kind = 'noun'
  AND enabled = TRUE
ORDER BY random() LIMIT 1;