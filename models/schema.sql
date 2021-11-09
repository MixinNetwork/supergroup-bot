CREATE TABLE IF NOT EXISTS activity (
  activity_index      SMALLINT NOT NULL PRIMARY KEY,
  client_id           VARCHAR(36) NOT NULL,
  status              SMALLINT DEFAULT 1, -- 1 不展示 2 展示
  img_url             VARCHAR(512) DEFAULT '',
  expire_img_url      VARCHAR(512) DEFAULT '',
  action              VARCHAR(512) DEFAULT '',
  start_at            TIMESTAMP WITH TIME ZONE NOT NULL,
  expire_at           TIMESTAMP WITH TIME ZONE NOT NULL,
  created_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
ADD INDEX IF NOT EXISTS activity_client_statusx ON activity(client_id, status);
