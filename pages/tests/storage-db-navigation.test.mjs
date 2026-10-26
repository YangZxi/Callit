import assert from "node:assert/strict";
import test from "node:test";
import { setImmediate } from "node:timers/promises";
import { createMemoryRouter } from "react-router-dom";

import { resolvePendingDBNavigation } from "../src/lib/storage-db-navigation.ts";

const dbPath = "/admin/storage/db";
const workerPath = "/admin/workers";
const kvPath = "/admin/storage/kv";

function createNavigationTestRouter(context, initialEntries = [workerPath, dbPath]) {
  const router = createMemoryRouter([{ path: "*", element: null }], { initialEntries, initialIndex: 1 });
  context.after(() => router.dispose());
  router.getBlocker("pending-deletes", () => true);
  return router;
}

test("取消站内导航保留当前页，确认后到达目标页", async (context) => {
  const router = createNavigationTestRouter(context);
  await router.navigate(kvPath);
  assert.equal(router.state.location.pathname, dbPath);
  const blocker = router.state.blockers.get("pending-deletes");
  assert.equal(blocker.state, "blocked");
  assert.equal(blocker.location.pathname, kvPath);
  resolvePendingDBNavigation(blocker, false, () => false);
  assert.equal(router.state.location.pathname, dbPath);
  assert.equal(router.state.blockers.get("pending-deletes").state, "unblocked");

  await router.navigate(kvPath);
  resolvePendingDBNavigation(router.state.blockers.get("pending-deletes"), false, () => true);
  await setImmediate();
  assert.equal(router.state.location.pathname, kvPath);
});

test("后退和前进均可取消或确认，不损坏历史位置", async (context) => {
  const router = createNavigationTestRouter(context, [workerPath, dbPath, kvPath]);
  for (const [delta, target] of [[-1, workerPath], [1, kvPath]]) {
    await router.navigate(delta);
    assert.equal(router.state.location.pathname, dbPath);
    resolvePendingDBNavigation(router.state.blockers.get("pending-deletes"), false, () => false);
    await setImmediate();
    assert.equal(router.state.location.pathname, dbPath);

    await router.navigate(delta);
    resolvePendingDBNavigation(router.state.blockers.get("pending-deletes"), false, () => true);
    await setImmediate();
    assert.equal(router.state.location.pathname, target);

    router.deleteBlocker("pending-deletes");
    await router.navigate(-delta);
    assert.equal(router.state.location.pathname, dbPath);
    router.getBlocker("pending-deletes", () => true);
  }
});

test("删除提交期间阻止离开，不展示放弃提示", async (context) => {
  const router = createNavigationTestRouter(context);
  await router.navigate(kvPath);
  resolvePendingDBNavigation(router.state.blockers.get("pending-deletes"), true, () => {
    assert.fail("提交中的删除不能提示放弃");
  });
  assert.equal(router.state.location.pathname, dbPath);
  assert.equal(router.state.blockers.get("pending-deletes").state, "unblocked");
});

test("无删除标记时允许正常导航", async (context) => {
  const router = createNavigationTestRouter(context);
  const blocker = router.getBlocker("pending-deletes", () => false);
  await router.navigate(kvPath);
  resolvePendingDBNavigation(blocker, false, () => {
    assert.fail("没有待删除数据时不应确认");
  });
  assert.equal(router.state.location.pathname, kvPath);
});
