export PATH="/data/dist/go1.3/bin:$PATH"
export GOTOOLDIR="/data/dist/go1.3/pkg/tool/linux_amd64"
export GOROOT="/data/dist/go1.3"
export mci_home="/data/mci"

cd $mci_home

./bin/hostinit -conf /data/home/etc/mci_settings.yml &
flock --close --nonblock monitor.lock ./bin/monitor -conf /data/home/etc/mci_settings.yml
flock --close --nonblock mci_repotracker.lock ./bin/repotracker -conf /data/home/etc/mci_settings.yml
flock --close --nonblock scheduler.lock ./bin/scheduler -conf /data/home/etc/mci_settings.yml
flock --close --nonblock task_runner.lock ./bin/taskrunner -conf /data/home/etc/mci_settings.yml
flock --close --nonblock mci_notify.lock ./bin/notify -conf /data/home/etc/mci_settings.yml
