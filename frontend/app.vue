<script lang="ts" setup>
  useHead({
    htmlAttrs: {
      dir: "rtl",
      lang: "fa",
    },
  });
if (useCookie("tarche")){
  const cookie = useState('cookie' , ()=>useCookie("tarche"))
}
</script>

<template>
  <div>
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </div>
</template>
