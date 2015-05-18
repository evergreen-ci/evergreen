#Evergreen
Evergreen is a homebrew continuous integration system built by MongoDB.
It utilizes multi-platform cloud computing to run tests as quickly as possible on as many platforms as possible.

###Features
--TODO Icon Images for each section and some screenshots
#####Elastic Computing
Evergreen was built to scale with load and provide optimal test parallelism.
When there are lots of commits coming in, Evergreen can spin up new hosts to run them; when the hosts are no longer needed, Evergreen shuts them down.
Use only what you need.

#####Multiplatform Support
Evergreen is built in Go, an awesome new systems programming from Google.
Your tests can run on any machine that Evergreen can cross-compile its agent for, so it can run on all of Go's myriad supported operating systems and architectures (TODO: link). 
Including
 1. Linux
 2. OSX
 3. Windows
 4. FreeBSD
 5. Solaris

on x86, amd64, and arm architectures. 

#####Precise, Actionable Feedback
No more guessing about who broken what.
By Default, Evergreen does not run every commit; however, in the event that something breaks, Evergreen will run failing tasks on previous commits to find the commit that caused the failure.
Think of it like a built-in `git bisect`.

Additionally, Evergreen provides multiple views of state of a build, so you can quickly determine what's breaking and when it started.

#####Patch Builds
You shouldn't have to wait until after you've committed to find out your changes break your tests.
Evergreen comes with the powerful feature of pre-commit patch builds, which allow you to submit uncommitted changes and run them against the variants and test suites you need.
These patch builds help alleviate the stress of multi-platform development and long-running tests by putting Evergreen's elastic, multi-platform computing right at your team's fingertips.

#####Easy Configuration
Evergreen is flexible.
If you can do it in a shell script, you can run it with Evergreen.
Internally, MongoDB uses Evergreen for everything from testing the main server, to compiling our documentation, creating Amazon AMIs, and even testing Evergreen itself.
Projects are defined with simple yaml config files, see HERE (TODO) for an example.

###How It Works
Evergreen monitors a set of github repositories, waiting for new commits. 
When a new commit comes in or enough time has passed, Evergreen schedules builds for different variants (different OSes, compile flags, etc).
Once builds are scheduled, an internal heuristic decides how many cloud machines to allocate in order to get tests done and quickly and cheaply as possible.

--TODO: Diagram Image

Evergreen sends a executable agent binary to each test machine, which begins running its given task.
Logs are streamed back to the main server, along with system statistics and formatted test results.

