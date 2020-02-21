# Journald uploader

This sends journald data to a sumologic HTTP collector.
It supports a ```-n``` argument to specify a field to use as the name.
To avoid duplicate messages, it only sends recent events (defined by the window ```-w``` argument).
Set this to a very high value to make sure messages aren't missed, or a very low value to avoid possible duplicates.
