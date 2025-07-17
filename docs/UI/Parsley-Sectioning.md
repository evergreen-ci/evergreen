# Parsley Sectioning

## Overview

Parsley sectioning is toggleable from the "Log Viewing" tab of the "Parsley Settings" menu and is available for task logs only. Enabling sectioning will render your task log into expandable sections split up by function and command.

## Feature details

### Feature components

At the top of the log viewer, two buttons are available to "Open all sections" and "Close all sections". Clicking these buttons will expand and collapse every function and command section in the log file.

![Toggle All Sections](../images/toggle_all_sections.png)

Similarly there are "Open Subsections" and "Close Subsections" buttons available on individual function rows, which can be used to expand and collapse all commands that belong to the function.

![Toggle Subsections](../images/toggle_subsections.png)

Clicking on the caret will also open and close a single command or function section.

### Section status

A function containing a failing command is marked with a failing status icon whereas as passing function is marked with a passing icon.

![Passing Function](../images/passing_function.png)

![Failing Function](../images/failing_function.png)

### Jump to failing line

Jump to Failing Line is a separate feature that works synergistically with Sectioning. When enabled with Sectioning, Parsley will open the failing command section and scroll to the failing line during initial load.

### Searching

Sections that match the search criteria will open automatically.

### Filtering

Sections will temporarily go away when filters are applied and come back when filters are removed.

### Shareline

When a link with a share line is opened, Parsley will open the section that contains the share line and scroll to the line. The failing line section will still open but Parsley will scroll to the share line instead.
