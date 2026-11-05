import React from "react";
import ReactDOM from "react-dom/client";
import "./styles.css";
import { readToken } from "@/lib/api";
import App from "./App";

readToken();
try {
  const t = localStorage.getItem("tock-theme");
  if (t === "light" || t === "dark") document.documentElement.dataset.theme = t;
} catch {
  /* blocked */
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
