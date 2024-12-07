import {Skeleton} from "@mantine/core";

export function FilesSkeleton() {
  const files = new Array(10).fill(1).map((_, i) => i);
  return files.map((file) => {
    return <Skeleton key={file} width={100} height={100} />;
  });
}
