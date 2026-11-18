// One-time repair after #10976 and #10985.
// Load in mongosh, then call backfillRepoPermissionScopes(db) to preview.
// After review, pause attachment, detachment, and
// restriction changes, then call backfillRepoPermissionScopes(db, {
//   apply: true, writesPaused: true
// }) using a primary connection with write access. MANA reconciliation alone
// does not cover all writers. Keep the printed plan as the before/after record.

function planRepoPermissionScopeBackfill(repos, projects, scopes) {
  function validate(document) {
    if (typeof document._id !== "string" || !document._id) {
      throw new Error("Expected a nonempty string resource ID");
    }
    if (document.restricted != null && typeof document.restricted !== "boolean") {
      throw new Error(`Invalid restriction flag on ${document._id}`);
    }
  }

  const reposById = new Map();
  const expected = new Map();
  for (const repo of repos) {
    validate(repo);
    reposById.set(repo._id, repo);
    expected.set(`admin_repo_${repo._id}`, new Set([repo._id]));
    expected.set(`unrestricted_branches_${repo._id}`, new Set());
  }

  // Hidden and disabled attached projects belong in these scopes too.
  for (const project of projects) {
    if (!project.repo_ref_id) continue;
    validate(project);
    const repo = reposById.get(project.repo_ref_id);
    if (!repo) throw new Error(`Project ${project._id} refers to a missing repo`);
    expected.get(`admin_repo_${repo._id}`).add(project._id);
    if (!(project.restricted ?? repo.restricted ?? false)) {
      expected.get(`unrestricted_branches_${repo._id}`).add(project._id);
    }
  }

  const scopesById = new Map(scopes.map((scope) => [scope._id, scope]));
  const changes = [];
  for (const [scopeId, wanted] of expected) {
    const scope = scopesById.get(scopeId);
    if (!scope || scope.type !== "project") {
      throw new Error(`Expected existing project-typed scope ${scopeId}`);
    }
    // Role-manager updates propagate to parents; these repo scopes are standalone.
    if (scope.parent != null && scope.parent !== "") {
      throw new Error(`Scope ${scopeId} has a parent; reconcile through the role manager`);
    }
    if (scope.resources != null && (!Array.isArray(scope.resources) ||
      scope.resources.some((resource) => typeof resource !== "string"))) {
      throw new Error(`Invalid resources on scope ${scopeId}`);
    }
    const present = new Set(scope.resources ?? []);
    const added = [...wanted].filter((id) => !present.has(id)).sort();
    const removed = [...present].filter((id) => !wanted.has(id)).sort();
    if (!added.length && !removed.length) continue;
    const prefix = scopeId.startsWith("admin_repo_") ? "admin_repo_" : "unrestricted_branches_";
    const repo = reposById.get(scopeId.slice(prefix.length));
    changes.push({
      repoId: repo._id,
      repository: `${repo.owner_name}/${repo.repo_name}`,
      scopeId,
      beforeResourcesPresent: Object.prototype.hasOwnProperty.call(scope, "resources"),
      before: scope.resources ?? null,
      after: [...wanted].sort(),
      added,
      removed,
    });
  }
  changes.sort((a, b) => a.scopeId.localeCompare(b.scopeId));
  return {
    summary: {
      reposChecked: repos.length,
      scopesChecked: expected.size,
      reposWithDrift: new Set(changes.map((change) => change.repoId)).size,
      scopesWithDrift: changes.length,
      membershipsToAdd: changes.reduce((count, change) => count + change.added.length, 0),
      membershipsToRemove: changes.reduce((count, change) => count + change.removed.length, 0),
    },
    changes,
  };
}

function readRepoPermissionScopeBackfillPlan(database) {
  const repos = database.repo_ref.find({}, {
    _id: 1, owner_name: 1, repo_name: 1, restricted: 1,
  }).maxTimeMS(30000).toArray();
  if (!repos.length) throw new Error("No repos found; check the connection");
  const projects = database.project_ref.find({}, {
    _id: 1, repo_ref_id: 1, restricted: 1,
  }).maxTimeMS(30000).toArray();
  const scopes = database.scopes.find({
    _id: /^(admin_repo_|unrestricted_branches_)/,
  }, { _id: 1, type: 1, parent: 1, resources: 1 }).maxTimeMS(30000).toArray();
  return planRepoPermissionScopeBackfill(repos, projects, scopes);
}

function backfillRepoPermissionScopes(database, { apply = false, writesPaused = false } = {}) {
  if (database.getName() !== "mci") throw new Error("Expected the Evergreen mci database");
  if (typeof apply !== "boolean" || typeof writesPaused !== "boolean") {
    throw new Error("apply and writesPaused must be booleans");
  }
  if (apply && !writesPaused) {
    throw new Error("Pause attachment, detachment, and restriction changes before applying");
  }
  if (apply) {
    const hello = database.runCommand({ hello: 1 });
    if (!hello.ok || (!hello.isWritablePrimary && hello.msg !== "isdbgrid")) {
      throw new Error("Apply requires a primary connection");
    }
    database.getMongo().setReadPref("primary");
  }

  const plan = readRepoPermissionScopeBackfillPlan(database);
  print(JSON.stringify({ mode: apply ? "apply" : "dry-run", ...plan }, null, 2));
  if (!apply) return plan;

  let scopesUpdated = 0;
  for (const change of plan.changes) {
    // Exact-array matching prevents overwriting a scope changed since planning.
    const result = database.scopes.updateOne({
      _id: change.scopeId,
      type: "project",
      resources: change.beforeResourcesPresent ? change.before : { $exists: false },
    }, { $set: { resources: change.after } }, {
      writeConcern: { w: "majority", wtimeout: 30000 },
    });
    if (result.matchedCount !== 1) {
      throw new Error(`Scope ${change.scopeId} changed during the run; stopped after ${scopesUpdated} updates`);
    }
    scopesUpdated += 1;
    print(JSON.stringify({ updatedScope: change.scopeId, scopesUpdated }));
  }

  const verification = readRepoPermissionScopeBackfillPlan(database);
  print(JSON.stringify({ scopesUpdated, verification: verification.summary }, null, 2));
  if (verification.changes.length) {
    throw new Error("Drift remains after the backfill; inspect before resuming writes");
  }
  return verification;
}
