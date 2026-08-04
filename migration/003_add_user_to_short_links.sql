ALTER TABLE short_links
ADD COLUMN user_id BIGINT;

ALTER TABLE short_links
ADD CONSTRAINT short_links_user_id_fk
FOREIGN KEY (user_id)
REFERENCES users(user_id)
ON DELETE SET NULL;

CREATE INDEX short_links_user_id_idx
ON short_links(user_id);