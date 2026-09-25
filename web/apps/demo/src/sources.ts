import jobApplicationYaml from "../../../../examples/openforms/forms/job-application.yaml?raw";
import hiringYaml from "../../../../examples/openforms/workflows/hiring.yaml?raw";

export interface SourceFile {
  file: string;
  code: string;
}

/** The exact files `openforms seed --demo` loads into the server. */
export const SOURCES: SourceFile[] = [
  { file: "forms/job-application.yaml", code: jobApplicationYaml },
  { file: "workflows/hiring.yaml", code: hiringYaml },
];
