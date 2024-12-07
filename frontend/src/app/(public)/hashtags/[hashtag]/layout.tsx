import {ReactNode} from "react";
import {Container, Stack, Title} from "@mantine/core";

type Props = {
  params: {
    hashtag?: string;
  };
  children: ReactNode;
};

function HashtagsDetailLayout({params, children}: Props) {
  return (
    <Container size="sm" mt={50}>
      <Title>#{decodeURI(params.hashtag ?? "")}</Title>
      <Stack gap={"md"} mt={"lg"}>
        {children}
      </Stack>
    </Container>
  );
}

export default HashtagsDetailLayout;
