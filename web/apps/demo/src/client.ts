import { OpenFormsClient } from "@openforms/sdk";

/** Same-origin client: the demo is served by the openforms server at /demo. */
export const client = new OpenFormsClient({ baseUrl: window.location.origin });
