const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const test = require("node:test");
const vm = require("node:vm");

const context = vm.createContext({ print() {} });
vm.runInContext(fs.readFileSync(path.join(__dirname, "backfill-repo-permission-scopes.js"), "utf8"), context);
const plan = context.planRepoPermissionScopeBackfill;
const run = context.backfillRepoPermissionScopes;
const clone = (value) => JSON.parse(JSON.stringify(value));

function fixture() {
  return {
    repos: [
      { _id: "restricted", owner_name: "org", repo_name: "a", restricted: true },
      { _id: "open", owner_name: "org", repo_name: "b" },
    ],
    projects: [
      { _id: "inherit", repo_ref_id: "restricted" },
      { _id: "nullInherit", repo_ref_id: "restricted", restricted: null },
      { _id: "overrideOpen", repo_ref_id: "restricted", restricted: false },
      { _id: "overrideRestricted", repo_ref_id: "open", restricted: true },
      { _id: "hidden", repo_ref_id: "open", hidden: true, enabled: false },
      { _id: "active", repo_ref_id: "open", enabled: true },
      { _id: "detached", repo_ref_id: "" },
    ],
    scopes: [
      { _id: "admin_repo_restricted", type: "project", resources: ["restricted", "inherit", "nullInherit", "overrideOpen"] },
      { _id: "unrestricted_branches_restricted", type: "project", resources: ["inherit", "nullInherit", "detached", "deleted"] },
      { _id: "admin_repo_open", type: "project", name: "Preserve metadata", resources: ["open", "overrideRestricted", "detached"] },
      { _id: "unrestricted_branches_open", type: "project", resources: ["active", "detached"] },
    ],
  };
}

function syntheticDatabase(data, { primary = true, beforeUpdate, verificationChange } = {}) {
  let reads = 0;
  const attempts = [];
  const cursor = (documents) => ({ maxTimeMS: () => ({ toArray: () => clone(documents) }) });
  return {
    getName: () => "mci",
    runCommand: () => ({ ok: 1, isWritablePrimary: primary }),
    getMongo: () => ({ setReadPref() {} }),
    repo_ref: { find: () => {
      reads += 1;
      if (reads === 2 && verificationChange) verificationChange(data);
      return cursor(data.repos);
    } },
    project_ref: { find: () => cursor(data.projects) },
    scopes: {
      find: () => cursor(data.scopes),
      updateOne(filter, update) {
        attempts.push(clone(filter));
        const scope = data.scopes.find((scope) => scope._id === filter._id);
        if (beforeUpdate) beforeUpdate(scope, attempts.length);
        if (scope.type !== filter.type || JSON.stringify(scope.resources) !== JSON.stringify(filter.resources)) {
          return { matchedCount: 0 };
        }
        scope.resources = clone(update.$set.resources);
        return { matchedCount: 1 };
      },
    },
    data,
    attempts,
  };
}

test("InheritedRestrictionsAndOverridesShouldProduceCorrectMembership", () => {
  const data = fixture();
  const changes = plan(data.repos, data.projects, data.scopes).changes;
  assert.deepEqual(clone(changes.find((change) => change.scopeId === "unrestricted_branches_restricted").after), ["overrideOpen"]);
  assert.deepEqual(clone(changes.find((change) => change.scopeId === "unrestricted_branches_open").after), ["active", "hidden"]);
  assert.deepEqual(clone(changes.find((change) => change.scopeId === "admin_repo_open").after), ["active", "hidden", "open", "overrideRestricted"]);
});

test("DetachedAndDeletedProjectsShouldBeRemovedAndMissingProjectsAdded", () => {
  const data = fixture();
  const result = plan(data.repos, data.projects, data.scopes);
  assert.equal(result.summary.scopesWithDrift, 3);
  assert.equal(result.summary.membershipsToAdd, 4);
  assert.equal(result.summary.membershipsToRemove, 6);
});

test("InvalidSourceRecordsAndUnexpectedScopesShouldAbortBeforeWrites", () => {
  for (const invalidate of [
    (data) => data.projects.push({ _id: "dangling", repo_ref_id: "absent" }),
    (data) => { data.repos[0].restricted = "false"; },
    (data) => { data.scopes[0].type = "distro"; },
    (data) => { data.scopes[0].parent = "parent"; },
    (data) => { data.scopes[0].resources = [123]; },
    (data) => data.scopes.shift(),
  ]) {
    const data = fixture();
    invalidate(data);
    const database = syntheticDatabase(data);
    assert.throws(() => run(database, { apply: true, writesPaused: true }));
    assert.equal(database.attempts.length, 0);
  }
});

test("DryRunShouldLeaveAllDataUnchanged", () => {
  const database = syntheticDatabase(fixture());
  const before = clone(database.data);
  assert.equal(run(database).summary.scopesWithDrift, 3);
  assert.equal(database.attempts.length, 0);
  assert.deepEqual(database.data, before);
});

test("ApplyShouldRequirePausedWritesAndPrimaryConnection", () => {
  const database = syntheticDatabase(fixture());
  assert.throws(() => run(database, { apply: true }), /Pause attachment/);
  assert.throws(() => run(database, { apply: "true" }), /must be booleans/);
  assert.throws(() => run(syntheticDatabase(fixture(), { primary: false }), { apply: true, writesPaused: true }), /primary connection/);
  assert.throws(() => run({ ...database, getName: () => "wrong" }), /Expected the Evergreen/);
  assert.equal(database.attempts.length, 0);
});

test("SuccessfulApplyShouldPreserveMetadataAndBeIdempotent", () => {
  const database = syntheticDatabase(fixture());
  const sources = clone({ repos: database.data.repos, projects: database.data.projects });
  assert.equal(run(database, { apply: true, writesPaused: true }).summary.scopesWithDrift, 0);
  assert.equal(database.attempts.length, 3);
  assert.equal(database.data.scopes.find((scope) => scope._id === "admin_repo_open").name, "Preserve metadata");
  assert.deepEqual({ repos: database.data.repos, projects: database.data.projects }, sources);
  assert.equal(run(database, { apply: true, writesPaused: true }).summary.scopesWithDrift, 0);
  assert.equal(database.attempts.length, 3);
});

test("ConcurrentScopeChangeShouldStopWithoutOverwriting", () => {
  const database = syntheticDatabase(fixture(), {
    beforeUpdate(scope, attempt) {
      if (attempt === 2) scope.resources.push("concurrent");
    },
  });
  assert.throws(() => run(database, { apply: true, writesPaused: true }), /stopped after 1 updates/);
  assert.equal(database.attempts.length, 2);
  assert.ok(database.data.scopes.some((scope) => scope.resources.includes("concurrent")));
});

test("VerificationShouldRejectRemainingDrift", () => {
  const database = syntheticDatabase(fixture(), {
    verificationChange(data) {
      data.projects.push({ _id: "future", repo_ref_id: "open" });
    },
  });
  assert.throws(() => run(database, { apply: true, writesPaused: true }), /Drift remains/);
});
