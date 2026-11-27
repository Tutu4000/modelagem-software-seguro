// Strict type-checked rules: no `any`, no unsafe use of untyped values, and
// nothing that turns user text into HTML or keeps tokens in web storage.
import js from "@eslint/js";
import { defineConfig } from "eslint/config";
import reactHooks from "eslint-plugin-react-hooks";
import globals from "globals";
import tseslint from "typescript-eslint";

export default defineConfig(
  { ignores: ["**/dist/**", "**/*.gen.ts", "e2e/report/**", "e2e/results/**", "demo/out/**", "demo/results/**"] },
  js.configs.recommended,
  tseslint.configs.strictTypeChecked,
  tseslint.configs.stylisticTypeChecked,
  {
    languageOptions: {
      parserOptions: { projectService: { allowDefaultProject: ["eslint.config.js"] }, tsconfigRootDir: import.meta.dirname },
      globals: { ...globals.browser },
    },
    plugins: { "react-hooks": reactHooks },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/no-unsafe-argument": "error",
      "@typescript-eslint/no-unsafe-assignment": "error",
      "@typescript-eslint/no-unsafe-call": "error",
      "@typescript-eslint/no-unsafe-member-access": "error",
      "@typescript-eslint/no-unsafe-return": "error",
      "@typescript-eslint/consistent-type-imports": "error",
      "no-restricted-syntax": [
        "error",
        {
          selector: "JSXAttribute[name.name='dangerouslySetInnerHTML']",
          message: "User text is rendered as text, never as HTML (stored XSS, C09).",
        },
        {
          selector: "AssignmentExpression[left.property.name=/^(innerHTML|outerHTML)$/]",
          message: "No HTML from strings: build the DOM with React.",
        },
      ],
      "no-restricted-globals": [
        "error",
        { name: "localStorage", message: "Tokens and personal data stay in memory (FRONTEND.md)." },
        { name: "sessionStorage", message: "Tokens and personal data stay in memory (FRONTEND.md)." },
      ],
    },
  },
  {
    files: ["e2e/**", "demo/**", "**/vite.config.ts", "vitest.config.ts", "playwright.config.ts", "playwright.demo.config.ts", "eslint.config.js"],
    languageOptions: { globals: { ...globals.node } },
  },
);
