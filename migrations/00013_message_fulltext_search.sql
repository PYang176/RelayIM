-- +goose Up

ALTER TABLE messages
    ADD FULLTEXT KEY ft_messages_content (content) WITH PARSER ngram;

-- +goose Down

ALTER TABLE messages
    DROP INDEX ft_messages_content;
