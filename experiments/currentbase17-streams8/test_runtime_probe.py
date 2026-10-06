import unittest

from runtime_probe import idle


class RuntimeIdleTest(unittest.TestCase):
    def test_all_lifetimes_must_finish(self):
        state = dict(active_requests=0, active_vm_ids=[], cleanup_tasks=0, pending_shim_refills=0,
                     pending_ws_cache_writes=0, active_uffd_handlers=0)
        self.assertTrue(idle(state))
        for key in state:
            busy = dict(state)
            busy[key] = ["vm"] if key == "active_vm_ids" else 1
            self.assertFalse(idle(busy), key)

    def test_old_probe_without_uffd_lifecycle_is_not_sufficient(self):
        with self.assertRaises(KeyError):
            idle(dict(active_requests=0, active_vm_ids=[], cleanup_tasks=0, pending_shim_refills=0, pending_ws_cache_writes=0))
