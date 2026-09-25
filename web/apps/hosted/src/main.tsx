import "@openforms/react/styles.css";
import "./hosted.css";
import { OpenFormsClient } from "@openforms/sdk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App.js";
import { parseRoute } from "./route.js";

const route = parseRoute(window.location.pathname, window.location.search);
const parent = window.parent !== window ? window.parent : null;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App route={route} client={new OpenFormsClient()} parent={parent} />
  </StrictMode>,
);
