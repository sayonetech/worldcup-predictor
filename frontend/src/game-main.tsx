import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { GoatGamePage } from "./components/GoatGamePage";
import "./styles/tokens.css";
import "./styles/v2-components.css";

const qc = new QueryClient();

ReactDOM.createRoot(document.getElementById("game-root")!).render(
  <React.StrictMode>
    <QueryClientProvider client={qc}>
      <GoatGamePage />
    </QueryClientProvider>
  </React.StrictMode>,
);
