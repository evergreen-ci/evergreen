// One-time repair of repo permission scope memberships after #10976 and #10985.
// See backfill-repo-permission-scopes.md for the full rollout procedure.
//
// Usage from mongosh:
//
//   load("scripts/backfill-repo-permission-scopes.js");
//
//   // Preview the changes. This only reads.
//   backfillRepoPermissionScopes(db);
//
//   // After reviewing the preview, pause project attachment, detachment, and
//   // restriction changes, then apply using a primary connection with write
//   // access. Pausing MANA reconciliation alone does not cover all writers.
//   backfillRepoPermissionScopes(db, { apply: true, writesPaused: true });
//
// Keep the printed plan as the before/after record.

function adminRepoScopeId(repoId) {
  return `admin_repo_${repoId}`;
}

function unrestrictedBranchesScopeId(repoId) {
  return `unrestricted_branches_${repoId}`;
}

// A branch's own restriction flag wins. If it's absent or null, the branch
// inherits the repo's flag.
function isBranchRestricted(project, repo) {
  return project.restricted ?? repo.restricted ?? false;
}

function validateRepoOrProject(document) {
  if (typeof document._id !== "string" || !document._id) {
    throw new Error("Expected a nonempty string resource ID");
  }
  if (document.restricted != null && typeof document.restricted !== "boolean") {
    throw new Error(`Invalid restriction flag on ${document._id}`);
  }
}

function validateRepoScope(scopeId, scope) {
  if (!scope || scope.type !== "project") {
    throw new Error(`Expected existing project-typed scope ${scopeId}`);
  }

  // The role manager propagates membership changes to parent scopes, but this
  // script writes to scopes directly. That's only safe for the standalone scopes
  // Evergreen creates for repos.
  if (scope.parent != null && scope.parent !== "") {
    throw new Error(`Scope ${scopeId} has a parent; reconcile through the role manager`);
  }

  const resources = scope.resources;
  const hasValidResources = resources == null ||
    (Array.isArray(resources) && resources.every((resource) => typeof resource === "string"));
  if (!hasValidResources) {
    throw new Error(`Invalid resources on scope ${scopeId}`);
  }
}

// Returns the members each repo scope should have, keyed by scope ID. A repo's
// admin scope contains the repo and every attached project. Its unrestricted
// branches scope contains only the attached projects that aren't restricted.
function computeDesiredScopeMembers(repos, projects) {
  const reposById = new Map();
  const desiredScopes = new Map();

  for (const repo of repos) {
    validateRepoOrProject(repo);
    reposById.set(repo._id, repo);
    desiredScopes.set(adminRepoScopeId(repo._id), { repo, members: new Set([repo._id]) });
    desiredScopes.set(unrestrictedBranchesScopeId(repo._id), { repo, members: new Set() });
  }

  // Hidden and disabled projects are deliberately included, matching how
  // Evergreen initializes these scopes.
  for (const project of projects) {
    const isAttachedToRepo = Boolean(project.repo_ref_id);
    if (!isAttachedToRepo) {
      continue;
    }

    validateRepoOrProject(project);
    const repo = reposById.get(project.repo_ref_id);
    if (!repo) {
      throw new Error(`Project ${project._id} refers to a missing repo`);
    }

    desiredScopes.get(adminRepoScopeId(repo._id)).members.add(project._id);
    if (!isBranchRestricted(project, repo)) {
      desiredScopes.get(unrestrictedBranchesScopeId(repo._id)).members.add(project._id);
    }
  }

  return desiredScopes;
}

// Compares the scopes against what the repos and projects say they should
// contain. Returns a summary and a list of the scopes that need to change. This
// doesn't touch the database, so it can be tested on plain objects.
function planRepoPermissionScopeBackfill(repos, projects, scopes) {
  const desiredScopes = computeDesiredScopeMembers(repos, projects);
  const scopesById = new Map(scopes.map((scope) => [scope._id, scope]));

  const changes = [];
  for (const [scopeId, { repo, members }] of desiredScopes) {
    const scope = scopesById.get(scopeId);
    validateRepoScope(scopeId, scope);

    const currentMembers = new Set(scope.resources ?? []);
    const added = [...members].filter((id) => !currentMembers.has(id)).sort();
    const removed = [...currentMembers].filter((id) => !members.has(id)).sort();
    if (added.length === 0 && removed.length === 0) {
      continue;
    }

    changes.push({
      repoId: repo._id,
      repository: `${repo.owner_name}/${repo.repo_name}`,
      scopeId,
      beforeResourcesPresent: Object.prototype.hasOwnProperty.call(scope, "resources"),
      before: scope.resources ?? null,
      after: [...members].sort(),
      added,
      removed,
    });
  }
  changes.sort((a, b) => a.scopeId.localeCompare(b.scopeId));

  let membershipsToAdd = 0;
  let membershipsToRemove = 0;
  for (const change of changes) {
    membershipsToAdd += change.added.length;
    membershipsToRemove += change.removed.length;
  }

  return {
    summary: {
      reposChecked: repos.length,
      scopesChecked: desiredScopes.size,
      reposWithDrift: new Set(changes.map((change) => change.repoId)).size,
      scopesWithDrift: changes.length,
      membershipsToAdd,
      membershipsToRemove,
    },
    changes,
  };
}

function readRepoPermissionScopeBackfillPlan(database) {
  const queryTimeoutMS = 30000;

  const repos = database.repo_ref
    .find({}, { _id: 1, owner_name: 1, repo_name: 1, restricted: 1 })
    .maxTimeMS(queryTimeoutMS)
    .toArray();
  if (repos.length === 0) {
    throw new Error("No repos found; check the connection");
  }

  const projects = database.project_ref
    .find({}, { _id: 1, repo_ref_id: 1, restricted: 1 })
    .maxTimeMS(queryTimeoutMS)
    .toArray();

  const scopes = database.scopes
    .find(
      { _id: /^(admin_repo_|unrestricted_branches_)/ },
      { _id: 1, type: 1, parent: 1, resources: 1 },
    )
    .maxTimeMS(queryTimeoutMS)
    .toArray();

  return planRepoPermissionScopeBackfill(repos, projects, scopes);
}

function assertConnectedToPrimary(database) {
  const hello = database.runCommand({ hello: 1 });
  // A mongos reports "isdbgrid" instead of isWritablePrimary.
  const isPrimary = hello.isWritablePrimary || hello.msg === "isdbgrid";
  if (!hello.ok || !isPrimary) {
    throw new Error("Apply requires a primary connection");
  }
}

// Writes each planned change and returns how many scopes were updated. Stops at
// the first scope that no longer matches the plan.
function applyScopeChanges(database, changes) {
  let scopesUpdated = 0;

  for (const change of changes) {
    // Matching on the exact resources array seen during planning means a scope
    // that someone else changed in the meantime is left alone, not overwritten.
    const scopeUnchangedSincePlanning = {
      _id: change.scopeId,
      type: "project",
      resources: change.beforeResourcesPresent ? change.before : { $exists: false },
    };
    const result = database.scopes.updateOne(
      scopeUnchangedSincePlanning,
      { $set: { resources: change.after } },
      { writeConcern: { w: "majority", wtimeout: 30000 } },
    );
    if (result.matchedCount !== 1) {
      throw new Error(`Scope ${change.scopeId} changed during the run; stopped after ${scopesUpdated} updates`);
    }

    scopesUpdated += 1;
    print(JSON.stringify({ updatedScope: change.scopeId, scopesUpdated }));
  }

  return scopesUpdated;
}

// Prints and returns the backfill plan. Only writes when apply is true, in which
// case it also verifies that no drift remains afterward.
function backfillRepoPermissionScopes(database, { apply = false, writesPaused = false } = {}) {
  if (database.getName() !== "mci") {
    throw new Error("Expected the Evergreen mci database");
  }
  if (typeof apply !== "boolean" || typeof writesPaused !== "boolean") {
    throw new Error("apply and writesPaused must be booleans");
  }
  if (apply) {
    if (!writesPaused) {
      throw new Error("Pause attachment, detachment, and restriction changes before applying");
    }
    assertConnectedToPrimary(database);
    database.getMongo().setReadPref("primary");
  }

  const plan = readRepoPermissionScopeBackfillPlan(database);
  print(JSON.stringify({ mode: apply ? "apply" : "dry-run", ...plan }, null, 2));
  if (!apply) {
    return plan;
  }

  const scopesUpdated = applyScopeChanges(database, plan.changes);

  const verification = readRepoPermissionScopeBackfillPlan(database);
  print(JSON.stringify({ scopesUpdated, verification: verification.summary }, null, 2));
  if (verification.changes.length > 0) {
    throw new Error("Drift remains after the backfill; inspect before resuming writes");
  }
  return verification;
}
