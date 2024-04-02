<script lang="ts" setup>
if (useCookie("jwt")) {
  const cookie = useState('cookie' , () => useCookie("jwt"))
}
useHead({
  titleTemplate:(title)=>{
    return title ? ` طرح چه | ${ title } ` : "طرح چه"
  }
})
</script>

<template>
  <div>
    <NuxtLayout>
      <NuxtPage />
    </NuxtLayout>
  </div>
</template>
