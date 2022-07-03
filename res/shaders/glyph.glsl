//shader:vertex
#version 410

//aVertPos must be in the range [0,1]
layout(location=0) in vec3 aVertPos;
layout(location=1) in vec2 aUV0;
layout(location=2) in vec4 aVertColor;
layout(location=3) in vec3 aModelPos;
layout(location=4) in vec3 aModelScale;

out vec2 v2fUV0;
out vec4 v2fColor;
out vec3 v2fFragPos;

//MVP = Model View Projection
uniform mat4 projViewMat;
uniform vec2 sizeUV;

void main()
{
    mat4 modelMat = mat4(
        aModelScale.x, 0.0, 0.0, 0.0, 
        0.0, aModelScale.y, 0.0, 0.0, 
        0.0, 0.0, aModelScale.z, 0.0, 
        aModelPos.x, aModelPos.y, aModelPos.z, 1.0
    );

    v2fUV0 = aUV0 + sizeUV*aVertPos.xy;
    v2fColor = aVertColor;
    v2fFragPos = vec3(modelMat * vec4(aVertPos, 1.0));

    gl_Position = projViewMat * modelMat * vec4(aVertPos, 1.0);
}

//shader:fragment
#version 410

in vec4 v2fColor;
in vec2 v2fUV0;
in vec3 v2fFragPos;

out vec4 fragColor;

uniform sampler2D diffTex;

void main()
{
    vec4 texColor = texture(diffTex, v2fUV0);
    // if (texColor.r == 0)
    // {
    //     fragColor = vec4(0,1,0,0.25);
    // }
    // else
    {
        fragColor = vec4(v2fColor.rgb, texColor.r);
    }
} 
