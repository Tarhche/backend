import {ReactNode} from "react";
import {createTheme, MantineProvider, DirectionProvider} from "@mantine/core";
import {QueryClientProvider} from "@/components/query-client-provider";
import {vazir, roboto_mono} from "./fonts";

const theme = createTheme({
  fontFamily: vazir.style.fontFamily,
  fontFamilyMonospace: roboto_mono.style.fontFamily,
  headings: {
    fontFamily: vazir.style.fontFamily,
  },
});

type Props = {
  children: ReactNode;
};

export async function Providers({children}: Props) {
  return (
    <QueryClientProvider>
      <MantineProvider theme={theme} defaultColorScheme="auto">
        <DirectionProvider initialDirection="rtl">{children}</DirectionProvider>
      </MantineProvider>
    </QueryClientProvider>
  );
}
