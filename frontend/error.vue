<template>
  <div class="container">
    <div class="row">
      <div class="error d-flex justify-content-center align-items-center min-vh-100 flex-column">
        <div class="image-error">
          <img src="@/assets/images/error/404.svg" alt="error image">
        </div>
        <div class="error-content">
          <h1 class="text-center px-5 gap-1 d-flex flex-row-reverse text-white rounded-3 pt-2 shadow-sm">
            <span>{{ error.statusCode }}</span>
            <span>ERROR</span>
          </h1>
          <p class="text-center mt-4 lead">{{ error.message.split(":")[0] }}</p>
        </div>
        <div class="redirect w-50 ">
          <button class="btn btn-outline-primary lead text-capitalize shadow-sm mt-3 w-100" @click="clearError({redirect:'/'})">back to home</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
const error = useError()
</script>

<style scoped>
h1 {
  background-color: rgb(52, 152, 219);
}

h1 span:last-child{
  font-weight: lighter;
}

h1 span:first-child{
  font-weight: bolder;
}

p {
  color: rgb(52, 152, 219);
  font-size:30px;
}
</style>