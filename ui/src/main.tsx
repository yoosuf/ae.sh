import { createRoot } from "react-dom/client";
import { App } from "./App";
import { readSnapshot } from "./parseSnapshot";
import "./styles.css";

const root = document.getElementById("root");
if (!root) throw new Error("root element is missing");
createRoot(root).render(<App snapshot={readSnapshot()} />);
