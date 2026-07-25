import { expect, test } from "bun:test";
import { GET } from "@/app/api/v1/health/route";
test("public health does not require private service configuration", async () => {
  const response = GET(new Request("https://file.cheap/api/v1/health"));
  expect(response.status).toBe(200);
  expect(await response.json()).toEqual({ deployment: "public-site", status: "ok", version: "filecheap-site/2" });
});
