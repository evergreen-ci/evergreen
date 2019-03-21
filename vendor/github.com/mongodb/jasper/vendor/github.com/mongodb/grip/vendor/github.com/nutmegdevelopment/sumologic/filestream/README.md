# Filestream uploader

This watches a file and sends data to a sumologic HTTP collector.
It handles file rotation, and has a configureable buffer period to avoid making excessive numbers of requests per second.