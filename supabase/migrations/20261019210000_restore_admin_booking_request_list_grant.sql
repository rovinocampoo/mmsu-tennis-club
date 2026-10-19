-- The training-focus migration recreated this admin-only listing function but
-- accidentally granted it to the public booking API role instead of the
-- dedicated admin API role. Restore the established least-privilege grant.
REVOKE ALL ON FUNCTION public.list_pending_booking_requests_with_slots()
    FROM PUBLIC, anon, authenticated, service_role, mmsu_schedule_reader,
         mmsu_booking_api, mmsu_booking_admin_api;

GRANT EXECUTE ON FUNCTION public.list_pending_booking_requests_with_slots()
    TO mmsu_booking_admin_api;
