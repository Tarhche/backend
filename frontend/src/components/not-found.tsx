import Link from "next/link";
import {Title, Text, Button, Container, Group} from "@mantine/core";
import classes from "./not-found.module.css";

type Props = {
  title?: string;
  text?: string;
  anchorText?: string;
  anchorLink?: string;
};

export function NotFound({title, text, anchorLink, anchorText}: Props) {
  const defaultText =
    "متاسفانه آدرس اشتباهی را دنبال کرده اید یا صفحه ای که به دنبال آن بودید به یک آدرس دیگری منتقل شده";
  return (
    <Container className={classes.root}>
      <div className={classes.label}>404</div>
      <Title className={classes.title}>
        {title ?? "یه جای مخفی پیدا کردی!"}
      </Title>
      <Text c="dimmed" size="lg" ta="center" className={classes.description}>
        {text ?? defaultText}
      </Text>
      <Group justify="center">
        <Button
          variant="subtle"
          size="md"
          component={Link}
          href={anchorLink ?? "/"}
        >
          {anchorText ?? "منو به خانه ببر"}
        </Button>
      </Group>
    </Container>
  );
}
