import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";

import { AppRoutes } from "@/app/router";
import { AuthProvider } from "@/auth/auth-context";
import "@/styles.css";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error('Assay web root element "#root" was not found');
}

createRoot(rootElement).render(
  <BrowserRouter>
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  </BrowserRouter>,
);
