import {BRAND_NAME} from "@/constants/strings";

type Options = {
  withBrand: boolean;
};

type Args = [...string[], options: Options] | [...string[]];

export function buildTitle(...args: Args) {
  let title = "";
  const options = args[args.length - 1];
  const segments = args.filter((arg) => typeof arg === "string") as string[];

  segments.forEach((arg, index) => {
    title += ` ${arg}`;
    if (index < segments.length - 1) {
      title += " |";
    }
  });

  if (typeof options === "object" && options.withBrand) {
    title += ` | ${BRAND_NAME}`;
  }

  return title.trim();
}
