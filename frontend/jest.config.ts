import type {Config} from "jest";
import nextJest from "next/jest.js";

const createJestConfig = nextJest({
  dir: "./",
});

const config: Config = {
  coverageProvider: "v8",
  testEnvironment: "jest-environment-jsdom",
  moduleNameMapper: {
    "@/(.*)$": "<rootDir>/$1",
  },
  setupFilesAfterEnv: ["<rootDir>/jest.setup.ts"],
  preset: "ts-jest",
};

export default createJestConfig(config);
