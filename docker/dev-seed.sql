-- registers the compose agent so it does not need to run `agent -init`
INSERT INTO server (name, timezone) VALUES ('test', 'UTC');
