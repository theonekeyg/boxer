import type { SidebarsConfig } from "@docusaurus/plugin-content-docs";

const sidebar: SidebarsConfig = {
  apisidebar: [
    {
      type: "doc",
      id: "api/boxer-api",
    },
    {
      type: "category",
      label: "files",
      items: [
        {
          type: "doc",
          id: "api/download-a-file-from-the-file-store",
          label: "Download a file from the file store",
          className: "api-method get",
        },
        {
          type: "doc",
          id: "api/upload-a-file-to-the-file-store",
          label: "Upload a file to the file store",
          className: "api-method post",
        },
      ],
    },
    {
      type: "category",
      label: "system",
      items: [
        {
          type: "doc",
          id: "api/health-check",
          label: "Health check",
          className: "api-method get",
        },
      ],
    },
    {
      type: "category",
      label: "execution",
      items: [
        {
          type: "doc",
          id: "api/execute-a-command-in-a-sandboxed-container",
          label: "Execute a command in a sandboxed container",
          className: "api-method post",
        },
      ],
    },
  ],
};

export default sidebar.apisidebar;
