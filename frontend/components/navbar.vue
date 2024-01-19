<template>
    <nav class="topnav navbar navbar-expand-lg navbar-light bg-white fixed-top">
        <div class="container">
            <NuxtLink class="navbar-brand" to="/">
                <strong>Tarhche</strong>
            </NuxtLink>
            <button class="navbar-toggler collapsed" type="button">
                <span class="navbar-toggler-icon"></span>
            </button>
            <div class="navbar-collapse show collapse" id="navbarColor02" style="">
                <ul class="navbar-nav ml-auto d-flex align-items-center">
                    <li class="nav-item highlight">
                        <NuxtLink class="nav-link" to="/auth/login">Login</NuxtLink>
                    </li>
                </ul>
            </div>
        </div>
    </nav>
</template>