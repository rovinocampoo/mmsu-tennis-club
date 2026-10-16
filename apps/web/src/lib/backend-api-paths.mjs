const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

export function isBackendRouteAllowed(method, path) {
  if (method === "POST" && path.length === 1 && ["booking-requests", "booking-status"].includes(path[0])) return true;
  if (method === "GET" && path.length === 2 && path[0] === "admin" && ["booking-requests", "players", "coach-availability", "training-programs", "training-sessions", "approved-clients"].includes(path[1])) return true;
  if (method === "POST" && path.length === 2 && path[0] === "coach" && path[1] === "availability") return true;
  if (method === "GET" && path.length === 2 && path[0] === "coach" && path[1] === "availability") return true;
  if (method === "GET" && path.length === 2 && path[0] === "coach" && path[1] === "sessions") return true;
  if (method === "DELETE" && path.length === 3 && path[0] === "coach" && path[1] === "availability" && uuid.test(path[2])) return true;
  if (method === "POST" && path.length === 2 && path[0] === "admin" && path[1] === "training-sessions") return true;
  if (method === "PATCH" && path.length === 3 && path[0] === "admin" && path[1] === "training-sessions" && uuid.test(path[2])) return true;
  if (method === "PATCH" && path.length === 4 && path[0] === "admin" && path[1] === "training-sessions" && uuid.test(path[2]) && path[3] === "coach") return true;
  if (method === "POST" && path.length === 4 && path[0] === "admin" && path[1] === "booking-requests" && uuid.test(path[2]) && ["approve", "reject"].includes(path[3])) return true;
  return method === "POST" && path.length === 5 && path[0] === "admin" && path[1] === "booking-requests" && uuid.test(path[2]) && path[3] === "notification" && path[4] === "retry";
}
