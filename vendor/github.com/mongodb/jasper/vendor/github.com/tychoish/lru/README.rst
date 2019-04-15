=====================================
``lru`` -- File System LRU Cache Tool
=====================================

.. image:: https://travis-ci.org/tychoish/lru.svg?branch=master
    :target: https://travis-ci.org/tychoish/lru

Overview
--------

lru is a tool for pruning file system caches based on access time.

Use
---

Begin by downloading the package: ::

  go get -u github.com/tychoish/lru

Then, in your project import: ::

  import "github.com/tychoish/lru"

Create a cache. You can either instantiate the cache manually and add
objects directly using the cache API, or more likely with either the
`DirectoryContents <https://godoc.org/github.com/tychoish/lru#DirectoryContents>`_
or `TreeContents <https://godoc.org/github.com/tychoish/lru#TreeContents>`_
constructors. All three methods are below: ::

   cache, err := TreeContents(<path>)
   cache, err := DirectoryContents(<path>)

   cache := NewCache()

Internally the cache stores items as ``FileObject`` instances, which
track the size, update time and fully qualified path of the
object. You can add these objects to the cache using the ``Add()``
method, or just use the ``AddFile`` helper, as in: ::

  err := cache.AddFile(<path>)

The ``Size()`` and ``Count()`` methods provide access to the current
state of the cache and the ``Pop()`` method to remove the oldest item
in the cache, but you can also use the ``Prune()`` method to remove
the oldest files until the cache, until the cache reaches a specific
size: ::

  // cache.Prune(<int>, <[]string{}>, <bool>)

  err := cache.Prune(<maxSize>, []string{<exclusions>}, <dryRun>)

The exclusion parameter makes it possible to exclude matching files
from the cache as needed.

Timestamps
----------

lru ages items out of the cache on the basis of the *mtime*, or last
modification time. This is the most reliable timestamp available
given the differences in filesystem configuration: other timing
information is platform dependent, and not reliably maintained.

While *access time* (``atime``) seems a more likely candidate for an
LRU cache, it is common practice to mount most filesystems with the
"no-atime" option, for read-only file systems, the default "relatime"
(relative atime,) will commonly update the ``atime`` value is only
updated once a day.

The downside of using ``mtime``, is that normal access operations will
not update the timestamp, which means your cache access operations
should explicitly update the ``mtime`` of the file, using the
``touch`` command or an equivalent utility or operation.

Documentation
-------------

See the `godoc documentation
<https://godoc.org/github.com/tychoish/lru>`_.
